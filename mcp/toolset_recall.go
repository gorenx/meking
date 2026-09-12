package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/memory/activation"
)

type RecallProtocolResource struct {
	Name         string          `json:"protocol_name"`
	Version      string          `json:"protocol_version"`
	Language     string          `json:"language"`
	Digest       string          `json:"protocol_digest"`
	Prompt       string          `json:"prompt"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

type RecallProtocolRequest struct {
	Language string `json:"language"`
}

func (toolset *Toolset) getRecallEvaluationProtocol(ctx context.Context, _ protocol.CallToolRequest, request RecallProtocolRequest) (*protocol.CallToolResult, error) {
	value, found := toolset.Observations.Protocol(request.Language)
	if !found {
		return toolset.failure(ctx, activation.ErrProtocolUnsupported), nil
	}
	return successResult(recallProtocolResource(value)), nil
}

func recallProtocolResource(value activation.Protocol) RecallProtocolResource {
	return RecallProtocolResource{
		Name:         "recall-observation",
		Version:      value.Version(),
		Language:     value.Language(),
		Digest:       value.Digest(),
		Prompt:       value.Prompt(),
		InputSchema:  json.RawMessage(value.InputSchema()),
		OutputSchema: json.RawMessage(value.OutputSchema()),
	}
}

func (toolset *Toolset) registerRecall(protocolServer *server.MCPServer) {
	protocolServer.AddTool(protocol.NewTool(
		"get_recall_evaluation_protocol",
		protocol.WithDescription("Read the current natural-recall evaluation instructions and exact JSON output schema before evaluating. Returns the bilingual protocol for the requested language; does not invoke a model or modify memory. Use its output_schema as submit_recall_observation arguments."),
		protocol.WithString("language", protocol.Required(), protocol.Enum("zh-CN", "en"), protocol.Description("Language of the evaluation instructions")),
		protocol.WithRawOutputSchema(json.RawMessage(recallProtocolSchema)),
		protocol.WithReadOnlyHintAnnotation(true),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithOpenWorldHintAnnotation(false),
	), protocol.NewTypedToolHandler(toolset.getRecallEvaluationProtocol))
	tool := protocol.NewTool(
		"submit_recall_observation",
		protocol.WithDescription("First call get_recall_evaluation_protocol for the evaluation instructions and JSON format. Submit the judgment for the observation bound by the Host: status, grade or reason_code, rationale and existing evidence_refs. Requires Host binding outside tool arguments. Returns a storage receipt, not an evaluation or S/D/R."),
		func(tool *protocol.Tool) {
			tool.InputSchema = protocol.ToolInputSchema{}
			tool.RawInputSchema = json.RawMessage(recallSubmissionSchema)
		},
		outputSchema[RecallReceipt](),
		protocol.WithReadOnlyHintAnnotation(false),
		protocol.WithDestructiveHintAnnotation(false),
		protocol.WithIdempotentHintAnnotation(true),
		protocol.WithOpenWorldHintAnnotation(false),
	)
	protocolServer.AddTool(tool, protocol.NewTypedToolHandler(toolset.submitRecallObservation))
	for _, language := range []string{"zh-CN", "en"} {
		uri := "meking://recall/protocols/1/" + language
		protocolServer.AddResource(
			protocol.NewResource(
				uri,
				"recall-observation-"+language,
				protocol.WithMIMEType(resourceMIMEType),
			),
			func(ctx context.Context,
				request protocol.ReadResourceRequest,
			) ([]protocol.ResourceContents, error) {
				value, found := toolset.Observations.Protocol(language)
				if !found {
					return nil, fmt.Errorf("recall protocol %s is unavailable", language)
				}
				return resourceContents(
					request.Params.URI,
					recallProtocolResource(value),
				)
			})
	}
}

func (toolset *Toolset) submitRecallObservation(
	ctx context.Context,
	call protocol.CallToolRequest,
	assessment RecallAssessmentResult,
) (*protocol.CallToolResult, error) {
	request, err := recallSubmission(call, assessment)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	input, err := recallValue(request)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	bound, err := toolset.zoneContext(ctx, request.UserID, request.SessionID)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	receipt, err := toolset.Observations.Submit(bound, input)
	if err != nil {
		return toolset.failure(ctx, err), nil
	}
	return successResult(recallReceipt(receipt)), nil
}
