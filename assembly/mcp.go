package assembly

import (
	"errors"

	"github.com/memoria-space/meking/mcp"
)

func (service *Service) MCP() (*mcp.Toolset, error) {
	if service == nil {
		return nil, errors.New("create MCP Toolset: Project service is required")
	}
	return mcp.NewToolset(mcp.Dependencies{
		Logger:       service.logger,
		Zones:        service.zones,
		Memories:     service.memoryRecorder,
		Search:       service.memorySearcher,
		Versions:     service.versions,
		Evidence:     service.evidenceReader,
		Messages:     service.messages,
		Conflicts:    service.resolutions,
		Deletions:    service.deletions,
		Observations: service.observations,
	})
}
