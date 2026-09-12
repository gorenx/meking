package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProjectServiceInitialize(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	service := NewLocalProjectService()
	result, err := service.Initialize(context.Background(), InitializeProject{
		Root: root, CompletionModel: "gpt-test", EmbeddingModel: "embedding-test",
	})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	if result.Root != resolvedRoot {
		t.Fatalf("result root = %q, want %q", result.Root, resolvedRoot)
	}
	if got, want := len(result.Created), 2+len(DefaultPrompts()); got != want {
		t.Fatalf("created file count = %d, want %d", got, want)
	}
	if slices.Contains(result.Created, ".workspace-id") {
		t.Fatalf("created files = %v, must not contain .workspace-id", result.Created)
	}

	for _, directory := range []string{"input", "cache", "prompts"} {
		info, err := os.Stat(filepath.Join(root, directory))
		if err != nil {
			t.Fatalf("stat directory %q: %v", directory, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", directory)
		}
	}

	settingsData, err := os.ReadFile(filepath.Join(root, "settings.yaml"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	settings, err := ParseSettings(settingsData)
	if err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if got := settings.CompletionModels["default_completion_model"].Model; got != "gpt-test" {
		t.Fatalf("completion model = %q, want %q", got, "gpt-test")
	}
	if got := strings.Count(string(settingsData), "base_url: \"\""); got != 2 {
		t.Fatalf("generated settings base URL fields = %d, want one for each model", got)
	}

	envInfo, err := os.Stat(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}
	if got := envInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf(".env mode = %o, want 600", got)
	}
}

func TestProjectServiceRejectsInitializedProject(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	service := NewLocalProjectService()
	command := InitializeProject{
		Root: root, CompletionModel: "gpt-test", EmbeddingModel: "embedding-test",
	}
	if _, err := service.Initialize(context.Background(), command); err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	custom := []byte("custom settings\n")
	if err := os.WriteFile(filepath.Join(root, "settings.yaml"), custom, 0o644); err != nil {
		t.Fatalf("write custom settings: %v", err)
	}
	if _, err := service.Initialize(context.Background(), command); !errors.Is(err, ErrProjectAlreadyInitialized) {
		t.Fatalf("second Initialize() error = %v, want ErrProjectAlreadyInitialized", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "settings.yaml"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(data) != string(custom) {
		t.Fatalf("settings were changed without force: %q", data)
	}
}

func TestProjectServiceForceOverwritesManagedFiles(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	service := NewLocalProjectService()
	command := InitializeProject{
		Root: root, CompletionModel: "gpt-old", EmbeddingModel: "embedding-old",
	}
	if _, err := service.Initialize(context.Background(), command); err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=do-not-keep\n"), 0o600); err != nil {
		t.Fatalf("write custom .env: %v", err)
	}
	command.Force = true
	command.CompletionModel = "gpt-new"
	command.EmbeddingModel = "embedding-new"
	result, err := service.Initialize(context.Background(), command)
	if err != nil {
		t.Fatalf("forced Initialize() error = %v", err)
	}
	if !slices.Contains(result.Overwritten, "settings.yaml") || !slices.Contains(result.Overwritten, ".env") {
		t.Fatalf("overwritten files = %v, want settings.yaml and .env", result.Overwritten)
	}
	settingsData, err := os.ReadFile(filepath.Join(root, "settings.yaml"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	settings, err := ParseSettings(settingsData)
	if err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if got := settings.CompletionModels["default_completion_model"].Model; got != "gpt-new" {
		t.Fatalf("completion model = %q, want %q", got, "gpt-new")
	}
}

func TestProjectServicePreservesExistingPromptBeforeFirstInitialization(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	promptPath := filepath.Join(root, "prompts", "extract_graph.txt")
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("create prompt directory: %v", err)
	}
	const custom = "custom prompt\n"
	if err := os.WriteFile(promptPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("write custom prompt: %v", err)
	}

	result, err := NewLocalProjectService().Initialize(context.Background(), InitializeProject{
		Root: root, CompletionModel: "gpt-test", EmbeddingModel: "embedding-test",
	})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if !slices.Contains(result.Preserved, "prompts/extract_graph.txt") {
		t.Fatalf("preserved files = %v, want extract_graph prompt", result.Preserved)
	}
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	if string(data) != custom {
		t.Fatalf("prompt = %q, want custom content", data)
	}
}

func TestProjectServiceRejectsFileRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	if err := os.WriteFile(root, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	_, err := NewLocalProjectService().Initialize(context.Background(), InitializeProject{
		Root: root, CompletionModel: "gpt-test", EmbeddingModel: "embedding-test",
	})
	if err == nil {
		t.Fatal("Initialize() error = nil, want non-nil")
	}
}

func TestProjectServiceRejectsSymlinkedManagedDirectory(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create project root: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside directory: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "prompts")); err != nil {
		t.Skipf("create symbolic link: %v", err)
	}

	_, err := NewLocalProjectService().Initialize(context.Background(), InitializeProject{
		Root: root, CompletionModel: "gpt-test", EmbeddingModel: "embedding-test",
	})
	if err == nil {
		t.Fatal("Initialize() error = nil, want non-nil")
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatalf("read outside directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory contains files: %v", entries)
	}
}

func TestProjectServiceHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := filepath.Join(t.TempDir(), "project")
	_, err := NewLocalProjectService().Initialize(ctx, InitializeProject{
		Root: root, CompletionModel: "gpt-test", EmbeddingModel: "embedding-test",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Initialize() error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Project exists after pre-cancelled initialization: %v", statErr)
	}
}

func TestProjectPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, path := range []string{"", "..", filepath.Join("..", "outside"), filepath.Join(root, "absolute")} {
		if _, err := projectPath(root, path); err == nil {
			t.Fatalf("projectPath(%q) error = nil, want non-nil", path)
		}
	}
}
