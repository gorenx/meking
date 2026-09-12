package httpservice

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	protocol "github.com/mark3labs/mcp-go/mcp"
	communityevents "github.com/memoria-space/meking/community/integration"
	"github.com/memoria-space/meking/controlplane"
	corpussqlite "github.com/memoria-space/meking/corpus/adapter/sqlite"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	corpustext "github.com/memoria-space/meking/corpus/text"
	epochsqlite "github.com/memoria-space/meking/epoch/adapter/sqlite"
	epochevents "github.com/memoria-space/meking/epoch/integration"
	"github.com/memoria-space/meking/internal/sqlitepool"
	"github.com/memoria-space/meking/journal"
	journalsqlite "github.com/memoria-space/meking/journal/adapter/sqlite"
	knowledgeevents "github.com/memoria-space/meking/knowledge/integration"
	"github.com/memoria-space/meking/project"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	"github.com/memoria-space/meking/zone"
	zonesqlite "github.com/memoria-space/meking/zone/adapter/sqlite"
)

const indexIntegrationWait = 15 * time.Second

func TestRunCreatesUserRootAndSessionZoneForMCP(t *testing.T) {
	projectRoot := t.TempDir()
	models := newIndexModelServer(t)
	if _, err := project.NewLocalProjectService().Initialize(t.Context(), project.InitializeProject{
		Root: projectRoot, CompletionModel: "completion-test", EmbeddingModel: "embedding-test",
		CompletionBaseURL: models.URL + "/v1", EmbeddingBaseURL: models.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(projectRoot, ".env"),
		[]byte("MEKING_API_KEY=test-key\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	port := reserveHTTPTestPort(t)
	startHTTPTestService(t, projectRoot, port)
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	httpClient := &http.Client{Timeout: time.Second}
	waitForEndpoint(t, httpClient, baseURL+"/api/v1/zones")
	protocolClient, err := mcpclient.NewStreamableHttpClient(baseURL + "/api/v1/mcp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = protocolClient.Close() })
	if err := protocolClient.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	initialize := protocol.InitializeRequest{}
	initialize.Params.ProtocolVersion = protocol.LATEST_PROTOCOL_VERSION
	initialize.Params.ClientInfo = protocol.Implementation{Name: "http-session-test", Version: "1"}
	if _, err := protocolClient.Initialize(t.Context(), initialize); err != nil {
		t.Fatal(err)
	}
	sessionID, err := zone.NewID()
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallToolRequest{}
	request.Params.Name = "list_entity_conflicts"
	request.Params.Arguments = map[string]any{
		"user_id":    "user-1",
		"session_id": string(sessionID),
	}
	result, err := protocolClient.CallTool(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("list_entity_conflicts result = %#v", result)
	}

	response, err := httpClient.Get(baseURL + "/api/v1/zones/" + string(sessionID))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var session httpZoneDefinition
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || session.Role != string(zone.RoleChild) || session.ParentZoneID == nil {
		t.Fatalf("Session Zone status/definition = %d/%#v", response.StatusCode, session)
	}
	rootResponse, err := httpClient.Get(baseURL + "/api/v1/zones/" + *session.ParentZoneID)
	if err != nil {
		t.Fatal(err)
	}
	defer rootResponse.Body.Close()
	var root httpZoneDefinition
	if err := json.NewDecoder(rootResponse.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}
	if rootResponse.StatusCode != http.StatusOK || root.Role != string(zone.RoleRoot) || root.ID != *session.ParentZoneID {
		t.Fatalf("User Root Zone status/definition = %d/%#v", rootResponse.StatusCode, root)
	}
}

func TestRunAcceptsDocumentAndDispatchesText(t *testing.T) {
	root := t.TempDir()
	models := newIndexModelServer(t)
	if _, err := project.NewLocalProjectService().Initialize(t.Context(), project.InitializeProject{
		Root: root, CompletionModel: "completion-test", EmbeddingModel: "embedding-test",
		CompletionBaseURL: models.URL + "/v1", EmbeddingBaseURL: models.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/.env", []byte("MEKING_API_KEY=test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "project.sqlite")
	database, err := sqlitepool.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close Project read database: %v", err)
		}
	})
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := journalsqlite.NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	texts, err := corpussqlite.NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	port := reserveHTTPTestPort(t)
	runContext, cancel := context.WithCancel(t.Context())
	runResult := make(chan error, 1)
	go func() {
		runResult <- Run(runContext, Config{
			ProjectRoot: root, Address: "127.0.0.1", Port: port,
		}, io.Discard, io.Discard)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-runResult:
			if err != nil {
				t.Errorf("run HTTP service: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("HTTP service did not stop after cancellation")
		}
	})

	client := &http.Client{Timeout: 500 * time.Millisecond}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	rootZone := createHTTPZone(t, client, baseURL, nil)
	zoneID, err := zone.ParseID(rootZone.ID)
	if err != nil {
		t.Fatal(err)
	}
	childZone := createHTTPZone(t, client, baseURL, &zoneID)
	if childZone.Role != string(zone.RoleChild) || childZone.ParentZoneID == nil || *childZone.ParentZoneID != rootZone.ID {
		t.Fatalf("HTTP Child Zone = %#v", childZone)
	}
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	enableHTTPAutomaticPolicies(t, client, baseURL)
	uploadHTTPDocument(t, client, baseURL, zoneID, "source.txt", "first\nsecond")

	deadline := time.Now().Add(indexIntegrationWait)
	for {
		snapshot, err := countTexts(zoneTestContext(t, zoneID), texts, 3)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Text snapshot after HTTP acceptance = %#v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}

	waitForJournalEventCount[knowledgeevents.EntityVectorsIndexedV1](t, journalStore, zoneID, 1)
	waitForJournalEventCount[corpusevents.CorporaPreparedV1](t, journalStore, zoneID, 1)
	waitForJournalEventCount[communityevents.StructurePreparedV1](t, journalStore, zoneID, 1)
	waitForJournalEventCount[epochevents.PublishedV1](t, journalStore, zoneID, 1)
	afterCatalog := browseHTTPDocuments(t, client, baseURL, zoneID)
	if len(afterCatalog.Documents) != 1 || !afterCatalog.Documents[0].Selected {
		t.Fatalf("Document Catalog after submission = %#v", afterCatalog)
	}
}

func TestRunKeepsServingAfterRejectedExtractionResult(t *testing.T) {
	root := t.TempDir()
	models := newIndexModelServer(t)
	models.rejectExtraction.Store(true)
	if _, err := project.NewLocalProjectService().Initialize(t.Context(), project.InitializeProject{
		Root: root, CompletionModel: "completion-test", EmbeddingModel: "embedding-test",
		CompletionBaseURL: models.URL + "/v1", EmbeddingBaseURL: models.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/.env", []byte("MEKING_API_KEY=test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := sqlitepool.Open(filepath.Join(root, "project.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	zoneID := createHTTPTestZone(t, database)

	port := reserveHTTPTestPort(t)
	run := startHTTPTestService(t, root, port)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	enableHTTPAutomaticPolicies(t, client, baseURL)
	uploadHTTPDocument(t, client, baseURL, zoneID, "rejected.txt", "first\nsecond")

	select {
	case <-models.extractionCorrection:
	case <-time.After(indexIntegrationWait):
		t.Fatal("Knowledge Extraction correction was not requested")
	}
	waitForIncompleteExtraction(t, database, zoneID)
	var versions int
	var sources int
	var progress int
	if err := database.QueryRowContext(
		t.Context(),
		`SELECT
            (SELECT count(*) FROM knowledge_versions WHERE zone_id = ?),
            (SELECT count(*) FROM knowledge_sources WHERE zone_id = ?),
            (SELECT count(*) FROM knowledge_extraction_progress WHERE zone_id = ?)`,
		string(zoneID),
		string(zoneID),
		string(zoneID),
	).Scan(&versions, &sources, &progress); err != nil {
		t.Fatal(err)
	}
	if versions != 0 || sources != 0 || progress != 0 {
		t.Fatalf(
			"rejected extraction rows: versions=%d sources=%d progress=%d, want all zero",
			versions,
			sources,
			progress,
		)
	}
	select {
	case err := <-run.result:
		run.stopped.Store(true)
		t.Fatalf("HTTP service stopped after rejected extraction: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if calls := models.completions.Load(); calls != 3 {
		t.Fatalf("Knowledge Extraction completion calls = %d, want 3 without polling retry", calls)
	}
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	run.stop(t)
}

func TestRunResumesCommittedIndexWorkAfterRestart(t *testing.T) {
	root := t.TempDir()
	models := newIndexModelServer(t)
	models.blockEmbeddings.Store(true)
	if _, err := project.NewLocalProjectService().Initialize(t.Context(), project.InitializeProject{
		Root: root, CompletionModel: "completion-test", EmbeddingModel: "embedding-test",
		CompletionBaseURL: models.URL + "/v1", EmbeddingBaseURL: models.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/.env", []byte("MEKING_API_KEY=test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "project.sqlite")
	database, err := sqlitepool.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close Project recovery database: %v", err)
		}
	})
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := journalsqlite.NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	texts, err := corpussqlite.NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	zoneID := createHTTPTestZone(t, database)

	port := reserveHTTPTestPort(t)
	first := startHTTPTestService(t, root, port)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	enableHTTPAutomaticPolicies(t, client, baseURL)
	uploadHTTPDocument(t, client, baseURL, zoneID, "source.txt", "first\nsecond")
	waitForTextCount(t, texts, zoneID, 1)
	waitForJournalEventCount[knowledgeevents.PublishedV1](t, journalStore, zoneID, 1)
	select {
	case <-models.embeddingStarted:
	case <-time.After(indexIntegrationWait):
		t.Fatal("Semantic embedding did not block before restart")
	}
	completedModelCalls := models.completions.Load()
	first.stop(t)

	models.blockEmbeddings.Store(false)
	second := startHTTPTestService(t, root, port)
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	waitForJournalEventCount[epochevents.PublishedV1](t, journalStore, zoneID, 1)
	if calls := models.completions.Load(); calls != completedModelCalls {
		t.Fatalf("completion model calls after restart = %d, want committed count %d", calls, completedModelCalls)
	}
	second.stop(t)
}

func TestRunResumesReportGenerationAfterFailure(t *testing.T) {
	root := t.TempDir()
	models := newIndexModelServer(t)
	if _, err := project.NewLocalProjectService().Initialize(t.Context(), project.InitializeProject{
		Root: root, CompletionModel: "completion-test", EmbeddingModel: "embedding-test",
		CompletionBaseURL: models.URL + "/v1", EmbeddingBaseURL: models.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/.env", []byte("MEKING_API_KEY=test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "project.sqlite")
	database, err := sqlitepool.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := journalsqlite.NewStore(database, transactions)
	if err != nil {
		t.Fatal(err)
	}
	epochStore, err := epochsqlite.NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	corpusStore, err := corpussqlite.NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	zoneID := createHTTPTestZone(t, database)

	port := reserveHTTPTestPort(t)
	firstRun := startHTTPTestService(t, root, port)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	enableHTTPAutomaticPolicies(t, client, baseURL)
	uploadHTTPDocument(t, client, baseURL, zoneID, "source.txt", "first\nsecond")
	waitForTextCount(t, corpusStore, zoneID, 1)
	waitForJournalEventCount[epochevents.PublishedV1](t, journalStore, zoneID, 1)
	current, err := epochStore.Current(zoneTestContext(t, zoneID))
	if err != nil || current.ID <= 0 {
		t.Fatalf("Current Epoch before failure = (%#v, %v)", current, err)
	}
	waitForReadyReportPublication(t, database, zoneID, int64(current.ID))
	firstRun.stop(t)

	models.failCommunityReports.Store(true)
	failedRun := startHTTPTestService(t, root, port)
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	uploadHTTPDocument(t, client, baseURL, zoneID, "second.txt", "third\nfourth")
	waitForTextCount(t, corpusStore, zoneID, 2)
	select {
	case <-models.communityReportStarted:
	case <-time.After(indexIntegrationWait):
		t.Fatal("Community report failure was not reached")
	}
	runErr := failedRun.waitForFailure(t)
	if !strings.Contains(runErr.Error(), "generate Reports for Epoch") ||
		!strings.Contains(runErr.Error(), "status=503") {
		t.Fatalf("HTTP service failure = %v", runErr)
	}

	published, err := epochStore.Current(zoneTestContext(t, zoneID))
	if err != nil || published.ID <= current.ID {
		t.Fatalf("Current Epoch after Report failure = (%#v, %v), want newer than %#v", published, err, current)
	}
	waitForJournalEventCount[epochevents.PublishedV1](t, journalStore, zoneID, 2)

	models.failCommunityReports.Store(false)
	secondRun := startHTTPTestService(t, root, port)
	waitForHTTPRuntime(t, client, baseURL, zoneID)
	waitForReadyReportPublication(t, database, zoneID, int64(published.ID))
	recovered, err := epochStore.Current(zoneTestContext(t, zoneID))
	if err != nil || !recovered.Equal(published) {
		t.Fatalf("Current Epoch after Report recovery = (%#v, %v), want %#v", recovered, err, published)
	}
	secondRun.stop(t)
}

type indexModelServer struct {
	*httptest.Server
	completions            atomic.Int64
	embeddingCalls         atomic.Int64
	blockEmbeddings        atomic.Bool
	failCommunityReports   atomic.Bool
	rejectExtraction       atomic.Bool
	embeddingStarted       chan struct{}
	communityReportStarted chan struct{}
	extractionCorrection   chan struct{}
}

func newIndexModelServer(t *testing.T) *indexModelServer {
	t.Helper()
	models := &indexModelServer{
		embeddingStarted:       make(chan struct{}, 1),
		communityReportStarted: make(chan struct{}, 1),
		extractionCorrection:   make(chan struct{}, 1),
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/chat/completions":
			var input struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
				ResponseFormat struct {
					JSONSchema struct {
						Name string `json:"name"`
					} `json:"json_schema"`
				} `json:"response_format"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			content := `<|COMPLETE|>`
			switch input.ResponseFormat.JSONSchema.Name {
			case "knowledge_graph":
				completion := models.completions.Add(1)
				if models.rejectExtraction.Load() {
					for _, message := range input.Messages {
						if strings.Contains(message.Content, "Your previous result was rejected") {
							select {
							case models.extractionCorrection <- struct{}{}:
							default:
							}
							break
						}
					}
					content = `{"entities":[],"relations":[{"source":"ALPHA","target":"BETA","type":"","description":"missing type","weight":1}]}`
				} else if completion == 1 {
					content = `{"entities":[{"name":"ALPHA","type":"PERSON","aliases":[],"description":"Alpha description"},{"name":"BETA","type":"PERSON","aliases":[],"description":"Beta description"}],"relations":[{"source":"ALPHA","target":"BETA","type":"COLLABORATION","description":"Alpha works with Beta","weight":1}]}`
				} else {
					content = `{"entities":[{"name":"ALPHA","type":"PERSON","aliases":[],"description":"Alpha description"},{"name":"GAMMA","type":"PERSON","aliases":[],"description":"Gamma description"}],"relations":[{"source":"ALPHA","target":"GAMMA","type":"COLLABORATION","description":"Alpha works with Gamma","weight":1}]}`
				}
			case "community_report":
				if models.failCommunityReports.Load() {
					select {
					case models.communityReportStarted <- struct{}{}:
					default:
					}
					http.Error(writer, "community report unavailable", http.StatusServiceUnavailable)
					return
				}
				content = `{"title":"Alpha","summary":"Alpha summary","findings":[{"summary":"Alpha finding","explanation":"Alpha evidence"}],"rating":5,"rating_explanation":"Alpha rating"}`
			case "report_fragment":
				content = `{"content":"Alpha evidence"}`
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id": "chatcmpl-index", "object": "chat.completion", "created": 1,
				"model": "completion-test",
				"choices": []map[string]any{{
					"index": 0, "finish_reason": "stop",
					"message": map[string]any{"role": "assistant", "content": content},
				}},
				"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
			})
		case "/v1/embeddings":
			var input struct {
				Texts json.RawMessage `json:"input"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			var texts []string
			if err := json.Unmarshal(input.Texts, &texts); err != nil {
				var text string
				if err := json.Unmarshal(input.Texts, &text); err != nil {
					http.Error(writer, err.Error(), http.StatusBadRequest)
					return
				}
				texts = []string{text}
			}
			if models.blockEmbeddings.Load() && models.embeddingCalls.Add(1) > 1 {
				select {
				case models.embeddingStarted <- struct{}{}:
				default:
				}
				<-request.Context().Done()
				return
			}
			data := make([]map[string]any, len(texts))
			for index := range texts {
				data[index] = map[string]any{
					"object": "embedding", "index": index, "embedding": []float64{1, 0},
				}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"object": "list", "model": "embedding-test",
				"data":  data,
				"usage": map[string]any{"prompt_tokens": 1, "total_tokens": 1},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	models.Server = server
	t.Cleanup(server.Close)
	return models
}

func waitForJournalEventCount[T journal.EventBody](
	t *testing.T,
	store *journalsqlite.Store,
	zoneID zone.ID,
	want int,
) {
	t.Helper()
	key := journal.EventKeyFor[T]()
	deadline := time.Now().Add(indexIntegrationWait)
	for {
		events, err := store.ReadEntries(t.Context(), journal.ZoneID(zoneID), 0, 512)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, event := range events {
			if event.Type == key.Type && event.SchemaVersion == key.Version {
				count++
			}
		}
		if count >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Journal event %s v%d count = %d, want at least %d", key.Type, key.Version, count, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type httpServiceRun struct {
	cancel  context.CancelFunc
	result  <-chan error
	stopped atomic.Bool
}

func startHTTPTestService(t *testing.T, root string, port int) *httpServiceRun {
	t.Helper()
	runContext, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- Run(runContext, Config{
			ProjectRoot: root,
			Address:     "127.0.0.1",
			Port:        port,
		}, io.Discard, io.Discard)
	}()
	run := &httpServiceRun{cancel: cancel, result: result}
	t.Cleanup(func() { run.stop(t) })
	return run
}

func (run *httpServiceRun) stop(t *testing.T) {
	t.Helper()
	if run == nil || run.stopped.Swap(true) {
		return
	}
	run.cancel()
	select {
	case err := <-run.result:
		if err != nil {
			t.Errorf("run HTTP recovery service: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("HTTP recovery service did not stop after cancellation")
	}
}

func (run *httpServiceRun) waitForFailure(t *testing.T) error {
	t.Helper()
	if run == nil || run.stopped.Load() {
		t.Fatal("HTTP service run is not active")
	}
	select {
	case err := <-run.result:
		run.stopped.Store(true)
		if err == nil {
			t.Fatal("HTTP service stopped without the expected failure")
		}
		return err
	case <-time.After(indexIntegrationWait):
		run.stop(t)
		t.Fatal("HTTP service did not stop after Consumer failure")
		return nil
	}
}

func waitForTextCount(t *testing.T, texts *corpussqlite.Store, zoneID zone.ID, want int) {
	t.Helper()
	deadline := time.Now().Add(indexIntegrationWait)
	for {
		snapshot, err := countTexts(zoneTestContext(t, zoneID), texts, 3)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Text count = %d, want %d", snapshot, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func createHTTPTestZone(t *testing.T, database *sql.DB) zone.ID {
	t.Helper()
	store, err := zonesqlite.NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := zone.NewRootDefinition(httpTestZoneID, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(t.Context(), definition); err != nil {
		t.Fatal(err)
	}
	return definition.ID
}

func zoneTestContext(t *testing.T, id zone.ID) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func zoneURL(baseURL string, id zone.ID) string {
	return baseURL + "/api/v1/zones/" + string(id)
}

func countTexts(ctx context.Context, texts *corpussqlite.Store, limit int) (int, error) {
	page, err := texts.TextStore().List(ctx, corpustext.Page{Limit: limit})
	if err != nil {
		return 0, err
	}
	return len(page.Texts), nil
}

func reserveHTTPTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func waitForHTTPRuntime(
	t *testing.T,
	client *http.Client,
	baseURL string,
	zoneID zone.ID,
) {
	t.Helper()
	waitForEndpoint(t, client, zoneURL(baseURL, zoneID)+"/runtime")
}

func waitForEndpoint(t *testing.T, client *http.Client, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		request, err := http.NewRequestWithContext(
			t.Context(), http.MethodGet, endpoint, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("HTTP endpoint %q did not become ready: %v", endpoint, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func uploadHTTPDocument(
	t *testing.T,
	client *http.Client,
	baseURL string,
	zoneID zone.ID,
	name string,
	content string,
) httpDocumentReceipt {
	t.Helper()
	body, contentType := documentMultipart(t, name, "text/plain", content)
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, zoneURL(baseURL, zoneID)+"/documents", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", contentType)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var receipt httpDocumentReceipt
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || receipt.Status != documentUploadedStatus ||
		receipt.ZoneID != string(zoneID) || receipt.DocumentID == "" || receipt.ContentDigest == "" {
		t.Fatalf("upload status/receipt = %d/%#v", response.StatusCode, receipt)
	}
	return receipt
}

func createHTTPZone(
	t *testing.T,
	client *http.Client,
	baseURL string,
	parentID *zone.ID,
) httpZoneDefinition {
	t.Helper()
	requestBody := httpCreateZoneRequest{}
	if parentID != nil {
		value := string(*parentID)
		requestBody.ParentZoneID = &value
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		request, err := http.NewRequestWithContext(
			t.Context(), http.MethodPost, baseURL+"/api/v1/zones", bytes.NewReader(body),
		)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err == nil {
			var definition httpZoneDefinition
			decodeErr := json.NewDecoder(response.Body).Decode(&definition)
			response.Body.Close()
			if response.StatusCode == http.StatusCreated && decodeErr == nil {
				return definition
			}
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			t.Fatalf("create HTTP Zone status/body = %d/%#v", response.StatusCode, definition)
		}
		if time.Now().After(deadline) {
			t.Fatalf("create HTTP Zone: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func browseHTTPDocuments(
	t *testing.T,
	client *http.Client,
	baseURL string,
	zoneID zone.ID,
) httpDocumentCatalogPage {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodGet, zoneURL(baseURL, zoneID)+"/documents?limit=100", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var page httpDocumentCatalogPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || page.ZoneID != string(zoneID) {
		t.Fatalf("browse HTTP Documents status/page = %d/%#v", response.StatusCode, page)
	}
	return page
}

func enableHTTPAutomaticPolicies(
	t *testing.T,
	client *http.Client,
	baseURL string,
) {
	t.Helper()
	for _, action := range controlplane.Actions() {
		body, err := json.Marshal(httpPublishControlPolicy{
			Mode:           string(controlplane.Automatic),
			MinimumPending: 1,
			MaximumWait:    "1s",
		})
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			baseURL+"/api/v1/control/policies/"+string(action),
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var policy httpControlPolicy
		decodeErr := json.NewDecoder(response.Body).Decode(&policy)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil ||
			policy.Action != string(action) || policy.Mode != string(controlplane.Automatic) {
			t.Fatalf("publish %s Policy status/result = %d/%#v, %v", action, response.StatusCode, policy, decodeErr)
		}
	}
}

func waitForReadyReportPublication(
	t *testing.T,
	database *sql.DB,
	zoneID zone.ID,
	epochID int64,
) {
	t.Helper()
	deadline := time.Now().Add(indexIntegrationWait)
	for {
		var count int
		err := database.QueryRowContext(
			t.Context(),
			`SELECT count(*) FROM community_report_publications
			 WHERE zone_id = ? AND epoch_id = ? AND vectors_ready = 1`,
			string(zoneID),
			epochID,
		).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Report publication for Epoch %d did not recover", epochID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForIncompleteExtraction(
	t *testing.T,
	database *sql.DB,
	zoneID zone.ID,
) {
	t.Helper()
	deadline := time.Now().Add(indexIntegrationWait)
	for {
		var cursor uint64
		var eventTypePosition uint64
		err := database.QueryRowContext(
			t.Context(),
			`SELECT
				COALESCE((
					SELECT position FROM journal_consumer_positions
					WHERE zone_id = ? AND consumer_id = 'knowledge.extraction'
					  AND event_type = 'corpus.corpora_prepared'
				), 0),
				COALESCE((
					SELECT MAX(event_sequence) + 1 FROM journal_events
					WHERE zone_id = ? AND event_type = 'corpus.corpora_prepared'
				), 0)`,
			string(zoneID),
			string(zoneID),
		).Scan(&cursor, &eventTypePosition)
		if err == nil && cursor == 0 && eventTypePosition > 0 {
			return
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("incomplete Knowledge Extraction position = %d/%d, error %v", cursor, eventTypePosition, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func documentMultipart(
	t *testing.T,
	name string,
	mediaType string,
	content string,
) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	header.Set("Content-Type", mediaType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}
