package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrAgentRequest identifies an Agent call failure during Knowledge Extraction.
// Delivery adapters use it to report model failures without treating them as
// process-runtime failures.
var ErrAgentRequest = errors.New("Knowledge Extraction Agent request failed")

// gleaningDialogue is one model conversation that extends an initial
// extraction response with zero or more gleaning rounds.
type gleaningDialogue struct {
	stage               string
	continuationPrompt  string
	loopPrompt          string
	completionDelimiter string
	recordDelimiter     string
	maxGleanings        int
	invalidResponse     error
	schemaName          string
	schema              json.RawMessage
}

// extractionConversation retains the Agent-visible turns and the complete
// result assembled by the extraction dialogue. It allows the extraction
// domain to ask the Agent to correct a rejected result without moving
// protocol validation into the Agent client.
type extractionConversation struct {
	messages []CompletionMessage
	result   string
}

// rejectedResult identifies the exact result fragment rejected by an
// extraction protocol and the reason it cannot be accepted.
type rejectedResult struct {
	part   string
	reason string
	cause  error
}

func (result *rejectedResult) Error() string {
	return result.cause.Error()
}

func (result *rejectedResult) Unwrap() error {
	return result.cause
}

func rejectResult(part, reason string, cause error) error {
	return &rejectedResult{
		part:   strings.TrimSpace(part),
		reason: strings.TrimSpace(reason),
		cause:  cause,
	}
}

func (dialogue gleaningDialogue) extract(
	ctx context.Context,
	model CompletionModel,
	initialPrompt string,
) (extractionConversation, error) {
	messages := []CompletionMessage{{
		Role:    CompletionRoleUser,
		Content: initialPrompt,
	}}
	response, err := dialogue.complete(ctx, model, messages, true)
	if err != nil {
		return extractionConversation{}, err
	}
	records := dialogue.records(response.Content)
	messages = append(messages, CompletionMessage{
		Role:    CompletionRoleAssistant,
		Content: response.Content,
	})

	for gleaning := 0; gleaning < dialogue.maxGleanings; gleaning++ {
		messages = append(messages, CompletionMessage{
			Role:    CompletionRoleUser,
			Content: dialogue.continuationPrompt,
		})
		response, err = dialogue.complete(ctx, model, messages, true)
		if err != nil {
			return extractionConversation{}, err
		}
		extension := dialogue.records(response.Content)
		if extension != "" {
			if records != "" {
				records += dialogue.recordDelimiter
			}
			records += extension
		}
		messages = append(messages, CompletionMessage{
			Role:    CompletionRoleAssistant,
			Content: response.Content,
		})

		if gleaning >= dialogue.maxGleanings-1 {
			break
		}
		messages = append(messages, CompletionMessage{
			Role:    CompletionRoleUser,
			Content: dialogue.loopPrompt,
		})
		response, err = dialogue.complete(ctx, model, messages, false)
		if err != nil {
			return extractionConversation{}, err
		}
		messages = append(messages, CompletionMessage{
			Role:    CompletionRoleAssistant,
			Content: response.Content,
		})
		decision := strings.TrimSpace(response.Content)
		if decision != "Y" && decision != "N" {
			rejected := response.Content
			reason := "continuation decision must be Y or N"
			messages = append(messages, CompletionMessage{
				Role: CompletionRoleUser,
				Content: fmt.Sprintf(
					"Your previous result was rejected.\n"+
						"Rejected result part:\n%s\n"+
						"Reason:\n%s\n"+
						"Return Y or N only.",
					rejected,
					reason,
				),
			})
			response, err = dialogue.complete(ctx, model, messages, false)
			if err != nil {
				return extractionConversation{}, err
			}
			messages = append(messages, CompletionMessage{
				Role:    CompletionRoleAssistant,
				Content: response.Content,
			})
			decision = strings.TrimSpace(response.Content)
		}
		switch decision {
		case "Y":
		case "N":
			return extractionConversation{messages: messages, result: records}, nil
		default:
			return extractionConversation{}, fmt.Errorf(
				"%w: continuation decision must be Y or N after correction",
				dialogue.invalidResponse,
			)
		}
	}
	return extractionConversation{messages: messages, result: records}, nil
}

func (dialogue gleaningDialogue) correct(
	ctx context.Context,
	model CompletionModel,
	conversation extractionConversation,
	rejection *rejectedResult,
) (extractionConversation, error) {
	messages := append([]CompletionMessage(nil), conversation.messages...)
	messages = append(messages, CompletionMessage{
		Role: CompletionRoleUser,
		Content: fmt.Sprintf(
			"Your previous result was rejected because it does not conform to the required protocol.\n"+
				"Rejected result part:\n%s\n"+
				"Reason:\n%s\n"+
				"Return the complete corrected result using the required protocol only.",
			rejection.part,
			rejection.reason,
		),
	})
	response, err := dialogue.complete(ctx, model, messages, true)
	if err != nil {
		return extractionConversation{}, err
	}
	messages = append(messages, CompletionMessage{
		Role:    CompletionRoleAssistant,
		Content: response.Content,
	})
	return extractionConversation{
		messages: messages,
		result:   dialogue.records(response.Content),
	}, nil
}

func (dialogue gleaningDialogue) complete(
	ctx context.Context,
	model CompletionModel,
	messages []CompletionMessage,
	structuredResult bool,
) (CompletionResponse, error) {
	request := CompletionRequest{
		Messages: append([]CompletionMessage(nil), messages...),
	}
	if structuredResult {
		request.SchemaName = dialogue.schemaName
		request.Schema = append(json.RawMessage(nil), dialogue.schema...)
	}
	response, err := model.Complete(ctx, request)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("%w during %s: %w", ErrAgentRequest, dialogue.stage, err)
	}
	return response, nil
}

func (dialogue gleaningDialogue) records(content string) string {
	return strings.TrimSpace(strings.TrimSuffix(
		strings.TrimSpace(content),
		dialogue.completionDelimiter,
	))
}

func renderPrompt(template string, fields map[string]string) (string, error) {
	var rendered strings.Builder
	for _, value := range fields {
		rendered.Grow(len(value))
	}
	rendered.Grow(len(template))
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
				return "", errors.New("unmatched '{' in prompt")
			}
			end += index + 1
			name := template[index+1 : end]
			value, found := fields[name]
			if !found {
				return "", fmt.Errorf("unknown prompt field %q", name)
			}
			rendered.WriteString(value)
			index = end + 1
		case '}':
			if index+1 < len(template) && template[index+1] == '}' {
				rendered.WriteByte('}')
				index += 2
				continue
			}
			return "", errors.New("unmatched '}' in prompt")
		default:
			rendered.WriteByte(template[index])
			index++
		}
	}
	return rendered.String(), nil
}
