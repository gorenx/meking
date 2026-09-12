package assembly_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memoria-space/meking/analysis"
	"github.com/memoria-space/meking/assembly"
	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
	corpussqlite "github.com/memoria-space/meking/corpus/adapter/sqlite"
	"github.com/memoria-space/meking/corpus/document"
	corpustext "github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/internal/sqlitepool"
	"github.com/memoria-space/meking/project"
	projectlocal "github.com/memoria-space/meking/project/adapter/local"
	"github.com/memoria-space/meking/zone"
)

func serviceConfig(t *testing.T, root string) assembly.Config {
	t.Helper()
	return serviceConfigWithModelURL(t, root, "")
}

func serviceConfigWithModelURL(t *testing.T, root string, modelURL string) assembly.Config {
	t.Helper()
	command := project.InitializeProject{
		Root: root, CompletionModel: "completion-test", EmbeddingModel: "embedding-test",
	}
	if modelURL != "" {
		command.CompletionBaseURL = modelURL + "/v1"
		command.EmbeddingBaseURL = modelURL + "/v1"
	}
	if _, err := project.NewLocalProjectService().Initialize(t.Context(), command); err != nil {
		t.Fatalf("initialize Project: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".env"),
		[]byte("MEKING_API_KEY=test-key\n"),
		0o600,
	); err != nil {
		t.Fatalf("write Project model credentials: %v", err)
	}
	return assembly.Config{
		Root:   root,
		Logger: discardLogger(),
	}
}

func TestServicePersistsUserRootZone(t *testing.T) {
	projectRoot := t.TempDir()
	config := serviceConfig(t, projectRoot)
	first, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	root, err := first.Zones().Root(t.Context(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	restored, err := second.Zones().Root(t.Context(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != root.ID || restored.UserID != "user-1" || restored.Role != zone.RoleRoot {
		t.Fatalf("restored User Root = %#v, want %#v", restored, root)
	}
}

func TestServiceResumesCommittedDocumentAfterRestart(t *testing.T) {
	root := t.TempDir()
	config := serviceConfig(t, root)

	first, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	analysisCache := filepath.Join(root, "cache", "analysis-cache.sqlite")
	if _, err := os.Stat(analysisCache); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default Project analysis cache stat error = %v, want not exist", err)
	}
	zoneDefinition, err := first.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	zoneContext := bindZoneContext(t, zoneDefinition.ID)
	_, err = first.Documents().SubmitDocument(zoneContext, document.UploadCommand{
		Name: "source.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("  first\r\nsecond  "),
	})
	if err != nil {
		t.Fatal(err)
	}
	enableManualAction(t, first, controlplane.ConvertDocument)
	if _, err := first.ControlActions().InvokeManual(zoneContext, controlplane.ConvertDocument); err != nil {
		t.Fatal(err)
	}
	texts := openTextReader(t, root, config.Logger)
	beforeRestart, err := countTexts(zoneContext, texts, 2)
	if err != nil {
		t.Fatal(err)
	}
	if beforeRestart != 0 {
		t.Fatalf("Text snapshot before Dispatcher starts = %#v", beforeRestart)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("close Project Service: %v", err)
		}
	})
	restoredZone, err := second.Zones().Resolve(t.Context(), zoneDefinition.ID)
	if err != nil {
		t.Fatalf("resolve Zone after restart: %v", err)
	}
	if restoredZone != zoneDefinition {
		t.Fatalf("Zone after restart = %#v, want %#v", restoredZone, zoneDefinition)
	}
	zoneContext = bindZoneContext(t, zoneDefinition.ID)

	runContext, cancel := context.WithCancel(t.Context())
	runResult := make(chan error, 1)
	go func() { runResult <- second.Run(runContext) }()
	t.Cleanup(func() {
		cancel()
		if err := <-runResult; err != nil {
			t.Errorf("run Project Service: %v", err)
		}
	})
	if _, err := second.ControlActions().InvokeManual(zoneContext, controlplane.ConvertDocument); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		snapshot, err := countTexts(zoneContext, texts, 2)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Text snapshot after restart = %#v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}

}

func TestServiceExcludesSecondWriterForProject(t *testing.T) {
	config := serviceConfig(t, t.TempDir())
	first, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Errorf("close Project Service: %v", err)
		}
	})

	second, err := assembly.Open(t.Context(), config)
	if !errors.Is(err, projectlocal.ErrProjectServiceLocked) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("second Open error = %v, want %v", err, projectlocal.ErrProjectServiceLocked)
	}
}

func TestServiceRequiresAnalysisOnlyForSelectedProjectCapabilities(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		rich      bool
		sentences bool
	}{
		{name: "rich documents", rich: true},
		{name: "sentence boundaries", sentences: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			config := serviceConfig(t, root)
			writeAnalysisSettings(t, root, test.rich, test.sentences)
			service, err := assembly.Open(t.Context(), config)
			if service != nil {
				_ = service.Close()
			}
			var failure *analysis.Failure
			if !errors.As(err, &failure) || failure.Kind != analysis.FailureUnavailable {
				t.Fatalf("Open() error = %v, want unavailable Analysis failure", err)
			}
		})
	}
}

func TestProjectAnalysisSidecarComposition(t *testing.T) {
	if os.Getenv("MEKING_ANALYSIS_INTEGRATION") != "1" {
		t.Skip("set MEKING_ANALYSIS_INTEGRATION=1 to run the local Analysis composition")
	}
	command := strings.TrimSpace(os.Getenv("MEKING_ANALYSIS_COMMAND"))
	if command == "" {
		t.Fatal("MEKING_ANALYSIS_COMMAND is required")
	}
	root := t.TempDir()
	config := serviceConfig(t, root)
	config.AnalysisCommand = command
	writeAnalysisSettings(t, root, true, true)

	service, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	runContext, cancel := context.WithCancel(t.Context())
	runResult := make(chan error, 1)
	running := false
	t.Cleanup(func() {
		if running {
			cancel()
			if err := <-runResult; err != nil {
				t.Errorf("run Project Service: %v", err)
			}
		}
		if err := service.Close(); err != nil {
			t.Errorf("close Project Service: %v", err)
		}
	})
	cachePath := filepath.Join(root, "cache", "analysis-cache.sqlite")
	if info, err := os.Stat(cachePath); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("analysis cache %q: info=%v error=%v", cachePath, info, err)
	}
	zoneContext := createRootZoneContext(t, service)
	_, err = service.Documents().SubmitDocument(zoneContext, document.UploadCommand{
		Name: "source.html", MediaType: "text/html",
		Content: bytes.NewBufferString(
			"<html><head><title>Atlas</title></head><body>Beacon knowledge.</body></html>",
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	enableManualAction(t, service, controlplane.ConvertDocument)
	if _, err := service.ControlActions().InvokeManual(zoneContext, controlplane.ConvertDocument); err != nil {
		t.Fatal(err)
	}
	texts := openTextReader(t, root, config.Logger)
	running = true
	go func() { runResult <- service.Run(runContext) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		snapshot, err := countTexts(zoneContext, texts, 2)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Text snapshot after rich Document event = %#v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	runErr := <-runResult
	running = false
	if runErr != nil {
		t.Fatal(runErr)
	}
}

func enableManualAction(
	t *testing.T,
	service *assembly.Service,
	action controlplane.Action,
) {
	t.Helper()
	if _, err := service.ControlPolicies().Publish(
		t.Context(),
		controlapplication.PublishPolicyInput{
			Action:      action,
			Mode:        controlplane.Manual,
			PublishedAt: time.Now().UTC(),
		},
	); err != nil {
		t.Fatal(err)
	}
}

func writeAnalysisSettings(t *testing.T, root string, rich, sentences bool) {
	t.Helper()
	settings, err := project.DefaultSettings("completion-test", "embedding-test")
	if err != nil {
		t.Fatal(err)
	}
	if rich {
		settings.Input.Type = project.InputTypeRichDocuments
	}
	if sentences {
		settings.Chunking.Type = "sentence"
		settings.Chunking.Size = 0
		settings.Chunking.Overlap = 0
	}
	data, err := project.MarshalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func openTextReader(t *testing.T, root string, _ *slog.Logger) corpustext.Store {
	t.Helper()
	path := filepath.Join(root, "project.sqlite")
	database, err := sqlitepool.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close Text reader database: %v", err)
		}
	})
	store, err := corpussqlite.NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	return store.TextStore()
}

func countTexts(ctx context.Context, texts corpustext.Store, limit int) (int, error) {
	page, err := texts.List(ctx, corpustext.Page{Limit: limit})
	if err != nil {
		return 0, err
	}
	return len(page.Texts), nil
}

func createRootZoneContext(t *testing.T, service *assembly.Service) context.Context {
	t.Helper()
	definition, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return bindZoneContext(t, definition.ID)
}

func bindZoneContext(t *testing.T, id zone.ID) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func readProjectSettings(t *testing.T, root string) project.Settings {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	settings, err := project.ParseSettings(data)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
