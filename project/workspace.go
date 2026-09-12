package project

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// ErrProjectAlreadyInitialized reports a non-forced initialization over settings.yaml.
var ErrProjectAlreadyInitialized = errors.New("project already initialized")

// InitializeProject carries the intent to initialize a local Project root.
type InitializeProject struct {
	Root            string
	Force           bool
	CompletionModel string
	EmbeddingModel  string
	// CompletionBaseURL and EmbeddingBaseURL are persisted with their model configurations.
	// Empty values keep the provider's default endpoint.
	CompletionBaseURL string
	EmbeddingBaseURL  string
}

// ProjectResult describes materialized and preserved Project files.
type ProjectResult struct {
	Root        string
	Created     []string
	Overwritten []string
	Preserved   []string
}

// ProjectFile is one application-owned file in a Project specification.
type ProjectFile struct {
	Path     string
	Content  []byte
	Mode     uint32
	Preserve bool
}

// ProjectSpec is the complete filesystem-neutral Project plan.
type ProjectSpec struct {
	Root        string
	Force       bool
	Marker      string
	Directories []string
	Files       []ProjectFile
}

// ProjectMaterializer applies a Project plan to a storage boundary.
type ProjectMaterializer interface {
	Materialize(ctx context.Context, spec ProjectSpec) (ProjectResult, error)
}

// ProjectService implements local Project lifecycle use cases.
type ProjectService struct {
	materializer ProjectMaterializer
}

// NewProjectService constructs a Project service with its materializer port.
func NewProjectService(materializer ProjectMaterializer) *ProjectService {
	if materializer == nil {
		panic("project materializer is required")
	}
	return &ProjectService{materializer: materializer}
}

// NewLocalProjectService wires the local filesystem materializer.
func NewLocalProjectService() *ProjectService {
	return NewProjectService(NewOSProjectMaterializer())
}

// Initialize creates or force-refreshes a versioned local Project root.
func (s *ProjectService) Initialize(ctx context.Context, cmd InitializeProject) (ProjectResult, error) {
	if err := ctx.Err(); err != nil {
		return ProjectResult{}, err
	}
	if cmd.Root == "" {
		return ProjectResult{}, errors.New("project root is required")
	}

	settings, err := DefaultSettings(cmd.CompletionModel, cmd.EmbeddingModel)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("build default settings: %w", err)
	}
	completion := settings.CompletionModels[settings.Index.CompletionModelID]
	completion.BaseURL = cmd.CompletionBaseURL
	settings.CompletionModels[settings.Index.CompletionModelID] = completion
	embedding := settings.EmbeddingModels[settings.Index.EmbeddingModelID]
	embedding.BaseURL = cmd.EmbeddingBaseURL
	settings.EmbeddingModels[settings.Index.EmbeddingModelID] = embedding
	settingsYAML, err := MarshalSettings(settings)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("render default settings: %w", err)
	}

	files := []ProjectFile{
		{Path: "settings.yaml", Content: settingsYAML, Mode: 0o644},
		{Path: ".env", Content: []byte("MEKING_API_KEY=<API_KEY>\n"), Mode: 0o600},
	}
	for name, content := range DefaultPrompts() {
		files = append(files, ProjectFile{
			Path: "prompts/" + name, Content: []byte(content), Mode: 0o644,
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	result, err := s.materializer.Materialize(ctx, ProjectSpec{
		Root: cmd.Root, Force: cmd.Force, Marker: "settings.yaml",
		Directories: []string{"input", "cache", "prompts"},
		Files:       files,
	})
	if err != nil {
		return ProjectResult{}, fmt.Errorf("initialize project: %w", err)
	}
	return result, nil
}
