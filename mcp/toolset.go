// Package mcp exposes Meking memory applications through the Model Context Protocol.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/corpus/message"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/memory"
	"github.com/memoria-space/meking/memory/activation"
	"github.com/memoria-space/meking/project"
	"github.com/memoria-space/meking/zone"
)

type Dependencies struct {
	Logger       *slog.Logger
	Zones        *zone.Catalog
	Memories     *memory.Recorder
	Search       *memory.Searcher
	Versions     knowledge.CurrentVersions
	Evidence     provenance.EvidenceReader
	Messages     *message.Log
	Conflicts    *resolution.Application
	Deletions    *deletion.Application
	Observations *activation.Service
}

// Toolset is the stateless MCP Agent Memory surface.
type Toolset struct {
	Dependencies
}

func NewToolset(dependencies Dependencies) (*Toolset, error) {
	if dependencies.Logger == nil {
		return nil, errors.New("create MCP Toolset: Logger is required")
	}
	switch {
	case dependencies.Zones == nil:
		return nil, errors.New("create MCP Toolset: Zones are required")
	case dependencies.Memories == nil:
		return nil, errors.New("create MCP Toolset: Memories are required")
	case dependencies.Search == nil:
		return nil, errors.New("create MCP Toolset: Memory Search is required")
	case dependencies.Versions == nil:
		return nil, errors.New("create MCP Toolset: current Knowledge Versions are required")
	case dependencies.Evidence == nil:
		return nil, errors.New("create MCP Toolset: Evidence Reader is required")
	case dependencies.Messages == nil:
		return nil, errors.New("create MCP Toolset: Message Log is required")
	case dependencies.Conflicts == nil:
		return nil, errors.New("create MCP Toolset: Conflicts are required")
	case dependencies.Deletions == nil:
		return nil, errors.New("create MCP Toolset: Deletions are required")
	case dependencies.Observations == nil:
		return nil, errors.New("create MCP Toolset: Observations are required")
	}
	for _, language := range []string{"zh-CN", "en"} {
		if _, found := dependencies.Observations.Protocol(language); !found {
			return nil, fmt.Errorf("create MCP Toolset: recall protocol %s is required", language)
		}
	}
	return &Toolset{Dependencies: dependencies}, nil
}

func (toolset *Toolset) Server() *server.MCPServer {
	protocolServer := server.NewMCPServer(
		"Meking Memory",
		project.CurrentApplicationVersion(),
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, false),
		server.WithToolHandlerMiddleware(toolset.recoverToolPanic),
		server.WithResourceHandlerMiddleware(toolset.recoverResourcePanic),
		server.WithStrictInputSchemaDefault(),
		server.WithInputSchemaValidation(),
		server.WithOutputSchemaValidation(),
	)
	toolset.registerMemoryTools(protocolServer)
	toolset.registerConflictTools(protocolServer)
	toolset.registerDeletionTools(protocolServer)
	toolset.registerResources(protocolServer)
	toolset.registerRecall(protocolServer)
	return protocolServer
}

func (toolset *Toolset) zoneContext(
	ctx context.Context,
	userID string,
	sessionID string,
) (context.Context, error) {
	childZoneID, err := zone.ParseID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid session: %w", err)
	}
	root, err := toolset.Zones.Root(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("identify User Root Zone: %w", err)
	}
	child, err := toolset.Zones.EnsureChild(
		ctx,
		root.ID,
		childZoneID,
	)
	if err != nil {
		if errors.Is(err, zone.ErrParentConflict) {
			return nil, fmt.Errorf("%w: Session belongs to another User", errForbidden)
		}
		return nil, fmt.Errorf("invalid session: %w", err)
	}
	bound, err := zone.NewContext(ctx, child.ID)
	if err != nil {
		return nil, fmt.Errorf("bind Child Zone: %w", err)
	}
	return bound, nil
}
