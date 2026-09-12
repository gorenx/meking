// Package agent adapts external Agent results to Community's ReportModel port.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	provider "github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/community"
)

var (
	reportSchema = json.RawMessage(`{
        "type":"object",
        "additionalProperties":false,
        "required":["title","summary","findings","rating","rating_explanation"],
        "properties":{
            "title":{"type":"string"},
            "summary":{"type":"string"},
            "findings":{
                "type":"array",
                "items":{
                    "type":"object",
                    "additionalProperties":false,
                    "required":["summary","explanation"],
                    "properties":{
                        "summary":{"type":"string"},
                        "explanation":{"type":"string"}
                    }
                }
            },
            "rating":{"type":"number"},
            "rating_explanation":{"type":"string"}
        }
    }`)
	fragmentSchema = json.RawMessage(`{
        "type":"object",
        "additionalProperties":false,
        "required":["content"],
        "properties":{"content":{"type":"string"}}
    }`)
)

type completionClient interface {
	Complete(context.Context, provider.CompletionRequest) (provider.CompletionResponse, error)
}

type ReportModel struct {
	client completionClient
}

type reportFormat struct {
	name   string
	schema json.RawMessage
}

type reportCorrection struct {
	prompt   string
	rejected string
	reason   error
	format   reportFormat
}

func NewReportModel(client completionClient) (*ReportModel, error) {
	if client == nil {
		return nil, errors.New("create Community Agent ReportModel: client is required")
	}
	return &ReportModel{client: client}, nil
}

func (model *ReportModel) GenerateCommunityReport(
	ctx context.Context,
	request community.ReportModelRequest,
) (community.ReportDraft, error) {
	format := reportFormat{name: "community_report", schema: reportSchema}
	response, err := model.complete(ctx, request.Prompt, format)
	if err != nil {
		return community.ReportDraft{}, err
	}
	draft, err := decodeReport(response)
	if err != nil {
		response, err = model.correct(ctx, reportCorrection{
			prompt: request.Prompt, rejected: response, reason: err, format: format,
		})
		if err != nil {
			return community.ReportDraft{}, err
		}
		draft, err = decodeReport(response)
		if err != nil {
			return community.ReportDraft{}, fmt.Errorf("decode corrected Community Agent result: %w", err)
		}
	}
	return draft, nil
}

func (model *ReportModel) GenerateReportFragment(
	ctx context.Context,
	request community.ReportFragmentRequest,
) (community.ReportFragment, error) {
	format := reportFormat{name: "report_fragment", schema: fragmentSchema}
	response, err := model.complete(ctx, request.Prompt, format)
	if err != nil {
		return community.ReportFragment{}, err
	}
	fragment, err := decodeFragment(response)
	if err != nil {
		response, err = model.correct(ctx, reportCorrection{
			prompt: request.Prompt, rejected: response, reason: err, format: format,
		})
		if err != nil {
			return community.ReportFragment{}, err
		}
		fragment, err = decodeFragment(response)
		if err != nil {
			return community.ReportFragment{}, fmt.Errorf("decode corrected Community Agent fragment: %w", err)
		}
	}
	return fragment, nil
}

func (model *ReportModel) complete(
	ctx context.Context,
	prompt string,
	format reportFormat,
) (string, error) {
	response, err := model.client.Complete(ctx, provider.CompletionRequest{
		Messages: []provider.CompletionMessage{
			{Role: provider.CompletionRoleUser, Content: prompt},
		},
		ResponseFormat: provider.CompletionResponseJSONSchema,
		JSONSchema: &provider.JSONSchema{
			Name:       format.name,
			Definition: append(json.RawMessage(nil), format.schema...),
		},
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

func (model *ReportModel) correct(
	ctx context.Context,
	correction reportCorrection,
) (string, error) {
	response, err := model.client.Complete(ctx, provider.CompletionRequest{
		Messages: []provider.CompletionMessage{
			{Role: provider.CompletionRoleUser, Content: correction.prompt},
			{Role: provider.CompletionRoleAssistant, Content: correction.rejected},
			{
				Role: provider.CompletionRoleUser,
				Content: fmt.Sprintf(
					"Your previous result was rejected because it does not conform to the required JSON schema.\n"+
						"Rejected result:\n%s\nReason:\n%s\nReturn the complete corrected JSON object only.",
					correction.rejected,
					correction.reason.Error(),
				),
			},
		},
		ResponseFormat: provider.CompletionResponseJSONSchema,
		JSONSchema: &provider.JSONSchema{
			Name:       correction.format.name,
			Definition: append(json.RawMessage(nil), correction.format.schema...),
		},
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

type reportResult struct {
	Title             *string          `json:"title"`
	Summary           *string          `json:"summary"`
	Findings          *[]findingResult `json:"findings"`
	Rating            *float64         `json:"rating"`
	RatingExplanation *string          `json:"rating_explanation"`
}

type findingResult struct {
	Summary     *string `json:"summary"`
	Explanation *string `json:"explanation"`
}

func decodeReport(content string) (community.ReportDraft, error) {
	var result reportResult
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return community.ReportDraft{}, err
	}
	if err := requireJSONEnd(decoder); err != nil {
		return community.ReportDraft{}, err
	}
	switch {
	case result.Title == nil:
		return community.ReportDraft{}, errors.New("missing required field title")
	case result.Summary == nil:
		return community.ReportDraft{}, errors.New("missing required field summary")
	case result.Findings == nil:
		return community.ReportDraft{}, errors.New("missing required field findings")
	case result.Rating == nil:
		return community.ReportDraft{}, errors.New("missing required field rating")
	case result.RatingExplanation == nil:
		return community.ReportDraft{}, errors.New("missing required field rating_explanation")
	}
	findings := make([]community.ReportFinding, len(*result.Findings))
	for index, finding := range *result.Findings {
		if finding.Summary == nil {
			return community.ReportDraft{}, fmt.Errorf("finding %d missing required field summary", index)
		}
		if finding.Explanation == nil {
			return community.ReportDraft{}, fmt.Errorf("finding %d missing required field explanation", index)
		}
		findings[index] = community.ReportFinding{
			Summary:     *finding.Summary,
			Explanation: *finding.Explanation,
		}
	}
	return community.ReportDraft{
		Title:             *result.Title,
		Summary:           *result.Summary,
		Findings:          findings,
		Rating:            *result.Rating,
		RatingExplanation: *result.RatingExplanation,
	}, nil
}

func decodeFragment(content string) (community.ReportFragment, error) {
	var result struct {
		Content *string `json:"content"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return community.ReportFragment{}, err
	}
	if err := requireJSONEnd(decoder); err != nil {
		return community.ReportFragment{}, err
	}
	if result.Content == nil || strings.TrimSpace(*result.Content) == "" {
		return community.ReportFragment{}, errors.New("required field content must be non-empty")
	}
	return community.ReportFragment{Content: *result.Content}, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return fmt.Errorf("decode trailing content: %w", err)
	}
	return nil
}

var _ community.ReportModel = (*ReportModel)(nil)
