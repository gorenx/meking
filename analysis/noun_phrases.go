package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxNounPhraseTextUnitsPerRequest is the transport batch bound. Consumer
	// adapters may accept larger domain inputs but must split them before using
	// the versioned endpoint.
	MaxNounPhraseTextUnitsPerRequest = 64

	maxNounPhraseTextRunes      = 1_000_000
	maxNounPhraseRequestBytes   = 1 << 20
	maxNounPhrasesPerTextUnit   = 100_000
	maxNounPhraseRunes          = 4096
	maxNounPhraseExcludedNouns  = 4096
	maxNounPhraseExcludedRunes  = 256
	maxNounPhraseDelimiterRunes = 32
)

// NounPhraseExtractor identifies the installed analysis policy used for one
// batch. The value participates in result identity and must never be inferred
// from free-form model or language names.
type NounPhraseExtractor string

const (
	// NounPhraseExtractorRegexEnglish selects the bounded English policy.
	NounPhraseExtractorRegexEnglish NounPhraseExtractor = "regex_english"
)

// NounPhraseAnalyzerConfig contains the output-affecting policy sent with one
// batch. It has no paths, runtime download switches, or process settings.
type NounPhraseAnalyzerConfig struct {
	ExtractorType NounPhraseExtractor `json:"extractor_type"`
	ExcludeNouns  []string            `json:"exclude_nouns"`
	MaxWordLength int                 `json:"max_word_length"`
	WordDelimiter string              `json:"word_delimiter"`
}

// NounPhraseTextUnit is a transport-only input. ID aligns the response and
// does not grant the analysis process ownership of a persisted text unit.
type NounPhraseTextUnit struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// NounPhraseTextUnitResult returns stable unique phrase membership for one
// request item without constructing graph entities or relationships.
type NounPhraseTextUnitResult struct {
	ID      string   `json:"id"`
	Phrases []string `json:"phrases"`
}

// NounPhraseResponse is the complete versioned result for one bounded batch.
type NounPhraseResponse struct {
	ContractVersion int                        `json:"contract_version"`
	TextUnits       []NounPhraseTextUnitResult `json:"text_units"`
}

type nounPhraseRequest struct {
	ContractVersion int                      `json:"contract_version"`
	Analyzer        NounPhraseAnalyzerConfig `json:"analyzer"`
	TextUnits       []NounPhraseTextUnit     `json:"text_units"`
}

type nounPhraseCachePayload struct {
	Phrases [][]string `json:"phrases"`
}

// NounPhrases analyzes each input with one explicit configuration. The full
// response is validated before any consumer can observe partial membership.
func (c *Client) NounPhrases(
	ctx context.Context,
	textUnits []NounPhraseTextUnit,
	analyzer NounPhraseAnalyzerConfig,
) (NounPhraseResponse, error) {
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return NounPhraseResponse{}, unavailable(
			"noun_phrases", false, errors.New("analysis client is not configured"),
		)
	}
	if err := validateNounPhraseRequest(textUnits, analyzer); err != nil {
		return NounPhraseResponse{}, failed("noun_phrases_request", false, err)
	}
	configurationHash, err := cacheHash(analyzer)
	if err != nil {
		return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, err)
	}
	texts := make([]string, len(textUnits))
	for index, textUnit := range textUnits {
		texts[index] = textUnit.Text
	}
	inputHash, err := cacheHash(texts)
	if err != nil {
		return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, err)
	}
	cacheKey, cacheEnabled, err := c.cacheKey(CapabilityNounPhrases, configurationHash, inputHash)
	if err != nil {
		return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, err)
	}
	if cacheEnabled {
		var cached nounPhraseCachePayload
		found, err := c.readCache(ctx, cacheKey, CapabilityNounPhrases, &cached)
		if err != nil {
			return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, err)
		}
		if found {
			if len(cached.Phrases) != len(textUnits) {
				return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, errors.New("noun phrase cache text unit count is invalid"))
			}
			response := NounPhraseResponse{ContractVersion: ContractVersion, TextUnits: make([]NounPhraseTextUnitResult, len(textUnits))}
			for index, textUnit := range textUnits {
				response.TextUnits[index] = NounPhraseTextUnitResult{ID: textUnit.ID, Phrases: append([]string(nil), cached.Phrases[index]...)}
			}
			if err := response.Validate(textUnits); err != nil {
				return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, err)
			}
			if err := validateNounPhraseResponsePolicy(response, analyzer); err != nil {
				return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, err)
			}
			return response, nil
		}
	}
	body, err := json.Marshal(nounPhraseRequest{
		ContractVersion: ContractVersion, Analyzer: analyzer, TextUnits: textUnits,
	})
	if err != nil {
		return NounPhraseResponse{}, failed("noun_phrases_request", false, err)
	}
	if len(body) > maxNounPhraseRequestBytes {
		return NounPhraseResponse{}, failed(
			"noun_phrases_request", false,
			errors.New("noun phrase request exceeds configured limit"),
		)
	}
	payload, err := c.doJSON(ctx, http.MethodPost, "/v1/nlp/noun-phrases", body)
	if err != nil {
		return NounPhraseResponse{}, err
	}
	var response NounPhraseResponse
	if err := decodeSingleJSON(payload, &response); err != nil {
		return NounPhraseResponse{}, failed("noun_phrases_response", false, err)
	}
	if err := response.Validate(textUnits); err != nil {
		return NounPhraseResponse{}, failed("noun_phrases_response", false, err)
	}
	if err := validateNounPhraseResponsePolicy(response, analyzer); err != nil {
		return NounPhraseResponse{}, failed("noun_phrases_response", false, err)
	}
	if cacheEnabled {
		cached := nounPhraseCachePayload{Phrases: make([][]string, len(response.TextUnits))}
		for index, textUnit := range response.TextUnits {
			cached.Phrases[index] = append([]string(nil), textUnit.Phrases...)
		}
		if err := c.writeCache(ctx, cacheKey, CapabilityNounPhrases, cached); err != nil {
			return NounPhraseResponse{}, unavailable("noun_phrases_cache", false, err)
		}
	}
	return response, nil
}

func validateNounPhraseResponsePolicy(
	response NounPhraseResponse,
	analyzer NounPhraseAnalyzerConfig,
) error {
	switch analyzer.ExtractorType {
	case NounPhraseExtractorRegexEnglish:
		for textUnitIndex, result := range response.TextUnits {
			for phraseIndex, phrase := range result.Phrases {
				if phrase != strings.ToUpper(phrase) {
					return fmt.Errorf(
						"noun phrase response text unit %d phrase %d is not canonical uppercase",
						textUnitIndex, phraseIndex,
					)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported noun phrase extractor %q", analyzer.ExtractorType)
	}
}

// Validate proves version, positional identity, stable ordering, uniqueness,
// and bounded text before results cross into graph-domain code.
func (r NounPhraseResponse) Validate(textUnits []NounPhraseTextUnit) error {
	if r.ContractVersion != ContractVersion {
		return fmt.Errorf("noun phrase contract version %d is incompatible", r.ContractVersion)
	}
	if len(r.TextUnits) != len(textUnits) {
		return errors.New("noun phrase response text unit count does not match request")
	}
	for textUnitIndex, result := range r.TextUnits {
		if result.ID != textUnits[textUnitIndex].ID {
			return fmt.Errorf("noun phrase response text unit %d is not aligned", textUnitIndex)
		}
		if len(result.Phrases) > maxNounPhrasesPerTextUnit {
			return fmt.Errorf("noun phrase response text unit %d has too many phrases", textUnitIndex)
		}
		if !slices.IsSorted(result.Phrases) {
			return fmt.Errorf("noun phrase response text unit %d is not sorted", textUnitIndex)
		}
		for phraseIndex, phrase := range result.Phrases {
			if phraseIndex > 0 && phrase == result.Phrases[phraseIndex-1] {
				return fmt.Errorf("noun phrase response text unit %d has duplicate phrases", textUnitIndex)
			}
			if !validNounPhraseValue(phrase, maxNounPhraseRunes) {
				return fmt.Errorf("noun phrase response text unit %d has an invalid phrase", textUnitIndex)
			}
		}
	}
	return nil
}

func validateNounPhraseRequest(
	textUnits []NounPhraseTextUnit,
	analyzer NounPhraseAnalyzerConfig,
) error {
	if len(textUnits) == 0 || len(textUnits) > MaxNounPhraseTextUnitsPerRequest {
		return fmt.Errorf(
			"noun phrase request must contain between 1 and %d text units",
			MaxNounPhraseTextUnitsPerRequest,
		)
	}
	if err := validateNounPhraseAnalyzer(analyzer); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(textUnits))
	for index, textUnit := range textUnits {
		if textUnit.ID == "" || len(textUnit.ID) > 128 || strings.TrimSpace(textUnit.ID) != textUnit.ID {
			return fmt.Errorf("noun phrase text unit %d has invalid id", index)
		}
		if _, exists := seen[textUnit.ID]; exists {
			return fmt.Errorf("noun phrase text unit id %q is duplicated", textUnit.ID)
		}
		seen[textUnit.ID] = struct{}{}
		if !utf8.ValidString(textUnit.Text) || utf8.RuneCountInString(textUnit.Text) > maxNounPhraseTextRunes {
			return fmt.Errorf("noun phrase text unit %d has invalid text", index)
		}
	}
	return nil
}

func validateNounPhraseAnalyzer(analyzer NounPhraseAnalyzerConfig) error {
	if analyzer.ExtractorType != NounPhraseExtractorRegexEnglish {
		return fmt.Errorf("unsupported noun phrase extractor %q", analyzer.ExtractorType)
	}
	if analyzer.MaxWordLength <= 0 || analyzer.MaxWordLength > maxNounPhraseTextRunes {
		return errors.New("noun phrase maximum word length is invalid")
	}
	if !validNounPhraseValue(analyzer.WordDelimiter, maxNounPhraseDelimiterRunes) {
		return errors.New("noun phrase word delimiter is invalid")
	}
	if len(analyzer.ExcludeNouns) > maxNounPhraseExcludedNouns {
		return errors.New("noun phrase excluded noun list is too large")
	}
	for _, noun := range analyzer.ExcludeNouns {
		if !validNounPhraseValue(noun, maxNounPhraseExcludedRunes) {
			return errors.New("noun phrase excluded noun is invalid")
		}
	}
	return nil
}

func validNounPhraseValue(value string, maxRunes int) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
