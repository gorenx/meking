package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	maxSentenceDocuments    = 64
	maxSentenceTextRunes    = 1_000_000
	maxSentenceRequestBytes = 1 << 20
)

// SentenceDocument is transport input only. ID aligns responses without
// granting the sidecar ownership of a corpus Document or a readable path.
type SentenceDocument struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Language string `json:"language"`
}

// SentenceSpan uses inclusive Unicode code-point offsets rather than UTF-8
// byte offsets.
type SentenceSpan struct {
	Index     int `json:"index"`
	StartChar int `json:"start_char"`
	EndChar   int `json:"end_char"`
}

// SentenceDocumentResult aligns ordered spans to one request ID.
type SentenceDocumentResult struct {
	ID    string         `json:"id"`
	Spans []SentenceSpan `json:"spans"`
}

// SentenceResponse is the complete versioned response from one bounded batch.
type SentenceResponse struct {
	ContractVersion int                      `json:"contract_version"`
	Documents       []SentenceDocumentResult `json:"documents"`
}

type sentenceRequest struct {
	ContractVersion int                `json:"contract_version"`
	Documents       []SentenceDocument `json:"documents"`
}

type sentenceCachePayload struct {
	Spans [][]SentenceSpan `json:"spans"`
}

// Sentences returns ordered inclusive code-point ranges for each input. It
// validates the complete response before exposing it to a consumer adapter.
func (c *Client) Sentences(ctx context.Context, documents []SentenceDocument) (SentenceResponse, error) {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return SentenceResponse{}, unavailable("sentences", false, errors.New("analysis client is not configured"))
	}
	if err := validateSentenceDocuments(documents); err != nil {
		return SentenceResponse{}, failed("sentences_request", false, err)
	}
	texts := make([]string, len(documents))
	languages := make([]string, len(documents))
	for index, document := range documents {
		texts[index] = document.Text
		languages[index] = document.Language
	}
	configurationHash, err := cacheHash(languages)
	if err != nil {
		return SentenceResponse{}, unavailable("sentences_cache", false, err)
	}
	inputHash, err := cacheHash(texts)
	if err != nil {
		return SentenceResponse{}, unavailable("sentences_cache", false, err)
	}
	cacheKey, cacheEnabled, err := c.cacheKey(CapabilitySentences, configurationHash, inputHash)
	if err != nil {
		return SentenceResponse{}, unavailable("sentences_cache", false, err)
	}
	if cacheEnabled {
		var cached sentenceCachePayload
		found, err := c.readCache(ctx, cacheKey, CapabilitySentences, &cached)
		if err != nil {
			return SentenceResponse{}, unavailable("sentences_cache", false, err)
		}
		if found {
			response := SentenceResponse{ContractVersion: ContractVersion, Documents: make([]SentenceDocumentResult, len(documents))}
			for index, document := range documents {
				if index >= len(cached.Spans) {
					return SentenceResponse{}, unavailable("sentences_cache", false, errors.New("sentence cache document count is invalid"))
				}
				response.Documents[index] = SentenceDocumentResult{ID: document.ID, Spans: append([]SentenceSpan(nil), cached.Spans[index]...)}
			}
			if len(cached.Spans) != len(documents) {
				return SentenceResponse{}, unavailable("sentences_cache", false, errors.New("sentence cache document count is invalid"))
			}
			if err := response.Validate(documents); err != nil {
				return SentenceResponse{}, unavailable("sentences_cache", false, err)
			}
			return response, nil
		}
	}
	body, err := json.Marshal(sentenceRequest{
		ContractVersion: ContractVersion,
		Documents:       documents,
	})
	if err != nil {
		return SentenceResponse{}, failed("sentences_request", false, err)
	}
	if len(body) > maxSentenceRequestBytes {
		return SentenceResponse{}, failed("sentences_request", false, errors.New("sentence request exceeds configured limit"))
	}
	payload, err := c.doJSON(ctx, http.MethodPost, "/v1/nlp/sentences", body)
	if err != nil {
		return SentenceResponse{}, err
	}
	var response SentenceResponse
	if err := decodeSingleJSON(payload, &response); err != nil {
		return SentenceResponse{}, failed("sentences_response", false, err)
	}
	if err := response.Validate(documents); err != nil {
		return SentenceResponse{}, failed("sentences_response", false, err)
	}
	if cacheEnabled {
		cached := sentenceCachePayload{Spans: make([][]SentenceSpan, len(response.Documents))}
		for index, document := range response.Documents {
			cached.Spans[index] = append([]SentenceSpan(nil), document.Spans...)
		}
		if err := c.writeCache(ctx, cacheKey, CapabilitySentences, cached); err != nil {
			return SentenceResponse{}, unavailable("sentences_cache", false, err)
		}
	}
	return response, nil
}

// Validate proves version, input alignment, indices, bounds, and ordering.
func (r SentenceResponse) Validate(documents []SentenceDocument) error {
	if r.ContractVersion != ContractVersion {
		return fmt.Errorf("sentence contract version %d is incompatible", r.ContractVersion)
	}
	if len(r.Documents) != len(documents) {
		return errors.New("sentence response document count does not match request")
	}
	for documentIndex, result := range r.Documents {
		input := documents[documentIndex]
		if result.ID != input.ID {
			return fmt.Errorf("sentence response document %d is not aligned", documentIndex)
		}
		runeCount := utf8.RuneCountInString(input.Text)
		previousEnd := -1
		for spanIndex, span := range result.Spans {
			if span.Index != spanIndex {
				return fmt.Errorf("sentence response document %d has invalid span index", documentIndex)
			}
			if span.StartChar < 0 || span.EndChar < span.StartChar || span.EndChar >= runeCount {
				return fmt.Errorf("sentence response document %d has out-of-range span", documentIndex)
			}
			if span.StartChar <= previousEnd {
				return fmt.Errorf("sentence response document %d has overlapping spans", documentIndex)
			}
			previousEnd = span.EndChar
		}
	}
	return nil
}

func validateSentenceDocuments(documents []SentenceDocument) error {
	if len(documents) == 0 || len(documents) > maxSentenceDocuments {
		return fmt.Errorf("sentence request must contain between 1 and %d documents", maxSentenceDocuments)
	}
	seen := make(map[string]struct{}, len(documents))
	for index, document := range documents {
		if document.ID == "" || len(document.ID) > 128 || strings.TrimSpace(document.ID) != document.ID {
			return fmt.Errorf("sentence document %d has invalid id", index)
		}
		if _, exists := seen[document.ID]; exists {
			return fmt.Errorf("sentence document id %q is duplicated", document.ID)
		}
		seen[document.ID] = struct{}{}
		if !utf8.ValidString(document.Text) || utf8.RuneCountInString(document.Text) > maxSentenceTextRunes {
			return fmt.Errorf("sentence document %d has invalid text", index)
		}
		if !validSentenceLanguage(document.Language) {
			return fmt.Errorf("sentence document %d has invalid language", index)
		}
	}
	return nil
}

func validSentenceLanguage(language string) bool {
	if len(language) < 2 || len(language) > 32 {
		return false
	}
	for index, value := range language {
		if value >= 'a' && value <= 'z' || index > 0 && value == '_' {
			continue
		}
		return false
	}
	return true
}
