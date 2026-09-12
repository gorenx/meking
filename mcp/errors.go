package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/memory"
	"github.com/memoria-space/meking/memory/activation"
	"github.com/memoria-space/meking/zone"
)

var errForbidden = errors.New("forbidden")

type ToolFailure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (toolset *Toolset) failure(ctx context.Context, err error) *protocol.CallToolResult {
	failure := classifyFailure(err)
	if failure.Code == "internal_error" {
		toolset.Logger.ErrorContext(ctx, "MCP tool failed", slog.String("error", err.Error()))
	}
	return failureResult(failure)
}

func (toolset *Toolset) recoverToolPanic(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request protocol.CallToolRequest,
	) (result *protocol.CallToolResult, resultErr error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				toolset.Logger.ErrorContext(
					ctx,
					"MCP tool panicked",
					slog.String("tool", request.Params.Name),
					slog.String("panic_type", fmt.Sprintf("%T", recovered)),
					slog.String("stack", string(debug.Stack())),
				)
				result = failureResult(ToolFailure{
					Code:      "internal_error",
					Message:   "internal error",
					Retryable: true,
				})
				resultErr = nil
			}
		}()
		return next(ctx, request)
	}
}

func (toolset *Toolset) recoverResourcePanic(next server.ResourceHandlerFunc) server.ResourceHandlerFunc {
	return func(
		ctx context.Context,
		request protocol.ReadResourceRequest,
	) (result []protocol.ResourceContents, resultErr error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				toolset.Logger.ErrorContext(
					ctx,
					"MCP resource panicked",
					slog.String("uri", request.Params.URI),
					slog.String("panic_type", fmt.Sprintf("%T", recovered)),
					slog.String("stack", string(debug.Stack())),
				)
				result = nil
				resultErr = errors.New("internal_error: internal error")
			}
		}()
		return next(ctx, request)
	}
}

func classifyFailure(err error) ToolFailure {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ToolFailure{Code: "cancelled", Message: "request was cancelled", Retryable: true}
	case errors.Is(err, corpus.ErrCorpusStorageBusy), errors.Is(err, knowledge.ErrStorageBusy), errors.Is(err, activation.ErrStorageUnavailable):
		return ToolFailure{Code: "storage_unavailable", Message: "storage is temporarily unavailable", Retryable: true}
	case errors.Is(err, errForbidden):
		return ToolFailure{Code: "forbidden", Message: "session belongs to another User"}
	case errors.Is(err, zone.ErrInvalidUser):
		return ToolFailure{Code: "invalid_user", Message: "user_id must be a non-empty valid User identifier"}
	case errors.Is(err, zone.ErrNotFound), errors.Is(err, zone.ErrInvalidDefinition),
		errors.Is(err, zone.ErrParentRequired), errors.Is(err, zone.ErrChildDepth):
		return ToolFailure{Code: "invalid_session", Message: "session_id must be a canonical UUID identifying a Child Zone"}
	case errors.Is(err, message.ErrIdentityConflict):
		return ToolFailure{Code: "message_identity_conflict", Message: safeError(err)}
	case errors.Is(err, message.ErrNotFound):
		return ToolFailure{Code: "message_not_found", Message: "message was not found"}
	case errors.Is(err, memory.ErrAlreadyExists):
		return ToolFailure{Code: "memory_already_exists", Message: safeError(err)}
	case errors.Is(err, activation.ErrInvalidObservation):
		return ToolFailure{Code: "invalid_observation", Message: "observation or assessment violates the recall protocol"}
	case errors.Is(err, activation.ErrObservationConflict):
		return ToolFailure{Code: "observation_conflict", Message: "observation identity already has different content"}
	case errors.Is(err, activation.ErrProtocolUnsupported):
		return ToolFailure{Code: "protocol_unsupported", Message: "read the current recall protocol before evaluating"}
	case errors.Is(err, activation.ErrVersionChanged):
		return ToolFailure{Code: "version_changed", Message: "read current knowledge and re-evaluate the original recall before a new submission"}
	case errors.Is(err, activation.ErrTargetNotFound):
		return ToolFailure{Code: "knowledge_not_found", Message: "current knowledge is missing or deleted"}
	case errors.Is(err, activation.ErrOutOfOrder):
		return ToolFailure{Code: "observation_out_of_order", Message: "observation predates the last applied recall; do not alter its time"}
	case errors.Is(err, activation.ErrModelMismatch):
		return ToolFailure{Code: "model_mismatch", Message: "stored memory uses a different model; explicit migration is required"}
	case errors.Is(err, activation.ErrDataIntegrity):
		return ToolFailure{Code: "data_integrity", Message: "stored activation data is inconsistent"}
	case errors.Is(err, deletion.ErrKnowledgeInUse):
		return ToolFailure{Code: "knowledge_in_use", Message: safeError(err)}
	case errors.Is(err, memory.ErrInvalid), errors.Is(err, message.ErrInvalid),
		errors.Is(err, knowledge.ErrInvalidChange):
		return ToolFailure{Code: "invalid_memory", Message: safeError(err)}
	case errors.Is(err, resolution.ErrConflictChanged):
		return ToolFailure{Code: "conflict_changed", Message: "conflict changed; read it again", Retryable: true}
	case errors.Is(err, resolution.ErrConflictNotFound):
		return ToolFailure{Code: "conflict_not_found", Message: "pending conflict was not found"}
	case errors.Is(err, knowledge.ErrNotFound):
		return ToolFailure{Code: "knowledge_not_found", Message: "knowledge was not found"}
	default:
		var versionConflict *knowledge.VersionConflict
		if errors.As(err, &versionConflict) {
			return ToolFailure{Code: "version_changed", Message: safeError(err), Retryable: true}
		}
		return ToolFailure{Code: "internal_error", Message: "internal error", Retryable: true}
	}
}

func safeError(err error) string {
	const maximum = 300
	message := strings.ReplaceAll(err.Error(), "\x00", "")
	if len(message) > maximum {
		return message[:maximum] + "..."
	}
	return message
}

func successResult(value any) *protocol.CallToolResult {
	return protocol.NewToolResultStructuredOnly(value)
}

func failureResult(failure ToolFailure) *protocol.CallToolResult {
	encoded, err := json.Marshal(failure)
	if err != nil {
		encoded = []byte(`{"code":"internal_error","message":"internal error","retryable":true}`)
	}
	return &protocol.CallToolResult{
		Content: []protocol.Content{
			protocol.TextContent{Type: "text", Text: string(encoded)},
		},
		StructuredContent: failure,
		IsError:           true,
	}
}
