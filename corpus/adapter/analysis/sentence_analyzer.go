package analysisadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	sidecar "github.com/memoria-space/meking/analysis"
	"github.com/memoria-space/meking/corpus/textunits"
)

type SentenceClient interface {
	Sentences(context.Context, []sidecar.SentenceDocument) (sidecar.SentenceResponse, error)
}

type SentenceAnalyzer struct {
	client SentenceClient
}

func NewSentenceAnalyzer(client SentenceClient) (*SentenceAnalyzer, error) {
	if client == nil {
		return nil, errors.New("create SentenceAnalyzer: analysis client is required")
	}
	return &SentenceAnalyzer{client: client}, nil
}

func (a *SentenceAnalyzer) AnalyzeSentences(
	ctx context.Context,
	request textunits.SentenceBoundaryRequest,
) ([]textunits.SentenceBoundary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.client == nil {
		return nil, errors.New("SentenceAnalyzer is not configured")
	}
	inputDigest := sha256.Sum256([]byte(request.Text))
	documents := []sidecar.SentenceDocument{{
		ID:   "sha256:" + hex.EncodeToString(inputDigest[:]),
		Text: request.Text, Language: request.Language,
	}}
	response, err := a.client.Sentences(ctx, documents)
	if err != nil {
		return nil, err
	}
	if err := response.Validate(documents); err != nil {
		return nil, fmt.Errorf("validate sentence analysis response: %w", err)
	}
	spans := response.Documents[0].Spans
	boundaries := make([]textunits.SentenceBoundary, len(spans))
	for index, span := range spans {
		boundaries[index] = textunits.SentenceBoundary{
			StartChar: span.StartChar, EndChar: span.EndChar,
		}
	}
	return boundaries, nil
}
