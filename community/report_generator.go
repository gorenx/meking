package community

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ReportModel generates publishable Reports and grounded intermediate
// Fragments. Community adapters decode Agent results; Report generation owns
// evidence coverage and final validation.
type ReportModel interface {
	GenerateCommunityReport(ctx context.Context, request ReportModelRequest) (ReportDraft, error)
	GenerateReportFragment(ctx context.Context, request ReportFragmentRequest) (ReportFragment, error)
}

// ReportTokenCounter measures report context using the tokenizer selected in
// Report settings. Implementations must be safe for concurrent generation.
type ReportTokenCounter interface {
	Count(text string) (int, error)
}

// MarshalReportJSON renders the canonical structured representation persisted
// with each immutable Report.
func MarshalReportJSON(draft ReportDraft) (string, error) {
	payload := struct {
		Title             string          `json:"title"`
		Summary           string          `json:"summary"`
		Findings          []ReportFinding `json:"findings"`
		Rating            reportJSONFloat `json:"rating"`
		RatingExplanation string          `json:"rating_explanation"`
	}{
		Title: draft.Title, Summary: draft.Summary, Findings: draft.Findings,
		Rating: reportJSONFloat(draft.Rating), RatingExplanation: draft.RatingExplanation,
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(payload); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

type reportJSONFloat float64

func (value reportJSONFloat) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(float64(value))
	if err != nil {
		return nil, err
	}
	if !strings.ContainsAny(string(encoded), ".eE") {
		encoded = append(encoded, '.', '0')
	}
	return encoded, nil
}

// RenderFullReport renders the canonical Markdown representation persisted and
// embedded for Query.
func RenderFullReport(draft ReportDraft) string {
	sections := make([]string, 0, len(draft.Findings))
	for _, finding := range draft.Findings {
		sections = append(sections, "## "+finding.Summary+"\n\n"+finding.Explanation)
	}
	return "# " + draft.Title + "\n\n" + draft.Summary + "\n\n" + strings.Join(sections, "\n\n")
}

// RenderReportPrompt expands the two supported Report prompt fields and
// rejects malformed or unknown placeholders.
func RenderReportPrompt(template, inputText string, maxReportLength int) (string, error) {
	var rendered strings.Builder
	for index := 0; index < len(template); {
		switch template[index] {
		case '{':
			if index+1 < len(template) && template[index+1] == '{' {
				rendered.WriteByte('{')
				index += 2
				continue
			}
			end := strings.IndexByte(template[index+1:], '}')
			if end < 0 {
				return "", errors.New("unclosed '{' in prompt template")
			}
			end += index + 1
			field := template[index+1 : end]
			switch field {
			case "input_text":
				rendered.WriteString(inputText)
			case "max_report_length":
				rendered.WriteString(strconv.Itoa(maxReportLength))
			default:
				return "", fmt.Errorf("unsupported prompt field %q", field)
			}
			index = end + 1
		case '}':
			if index+1 < len(template) && template[index+1] == '}' {
				rendered.WriteByte('}')
				index += 2
				continue
			}
			return "", errors.New("single '}' in prompt template")
		default:
			rendered.WriteByte(template[index])
			index++
		}
	}
	return rendered.String(), nil
}

// ValidateReportPrompt checks the template grammar and fields without calling
// a model provider.
func ValidateReportPrompt(template string) error {
	_, err := RenderReportPrompt(template, "input", 1)
	return err
}
