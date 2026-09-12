package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModelCredentialsRemainFixedAfterLoad(t *testing.T) {
	root := t.TempDir()
	settings, err := DefaultSettings("completion", "embedding")
	if err != nil {
		t.Fatal(err)
	}
	completion := settings.CompletionModels[settings.Index.CompletionModelID]
	completion.APIKey = "${STARTUP_PROCESS_KEY}"
	settings.CompletionModels[settings.Index.CompletionModelID] = completion
	embedding := settings.EmbeddingModels[settings.Index.EmbeddingModelID]
	embedding.APIKey = "${STARTUP_PROJECT_KEY}"
	settings.EmbeddingModels[settings.Index.EmbeddingModelID] = embedding

	t.Setenv("STARTUP_PROCESS_KEY", "process-first")
	t.Setenv("STARTUP_PROJECT_KEY", "<API_KEY>")
	environmentPath := filepath.Join(root, ".env")
	if err := os.WriteFile(
		environmentPath,
		[]byte("STARTUP_PROCESS_KEY=project-ignored\nSTARTUP_PROJECT_KEY=project-first\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	credentials := loadModelCredentials(root)

	t.Setenv("STARTUP_PROCESS_KEY", "process-second")
	if err := os.WriteFile(
		environmentPath,
		[]byte("STARTUP_PROCESS_KEY=project-second\nSTARTUP_PROJECT_KEY=project-second\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	resolved, err := credentials.resolveSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.CompletionModels[settings.Index.CompletionModelID].APIKey; got != "process-first" {
		t.Fatalf("completion credential = %q, want startup process value", got)
	}
	if got := resolved.EmbeddingModels[settings.Index.EmbeddingModelID].APIKey; got != "project-first" {
		t.Fatalf("embedding credential = %q, want startup Project value", got)
	}
}
