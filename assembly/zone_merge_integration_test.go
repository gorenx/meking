package assembly_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/memoria-space/meking/assembly"
	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	corpusevents "github.com/memoria-space/meking/corpus/integration"
	epochevents "github.com/memoria-space/meking/epoch/integration"
	"github.com/memoria-space/meking/journal"
	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	"github.com/memoria-space/meking/zone"
	zonemerger "github.com/memoria-space/meking/zone/merger"
)

const zoneMergeIntegrationWait = 20 * time.Second

func TestZoneMergePublishesChildCorpusAndKnowledgeThroughParentFlow(t *testing.T) {
	root := t.TempDir()
	models := newAssemblyModelServer(t)
	config := serviceConfigWithModelURL(t, root, models.URL)
	service, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close assembly Service: %v", err)
		}
	})
	startZoneMergeDispatcher(t, service)

	parent, err := service.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext := bindZoneContext(t, parent.ID)
	childContext := bindZoneContext(t, child.ID)

	childPublished := indexZoneDocument(
		t,
		service,
		childContext,
		"zone-merge/child-1",
		"child.txt",
		"CHILD-BETA: Alpha works with Beta.",
		0,
	)
	childCorpora, err := service.Documents().Corpora(
		childContext,
		corpus.CorporaID(childPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	childEntities, err := service.Knowledge().BrowseEntities(
		childContext,
		queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEntityTitles(t, childEntities.Items, "ALPHA", "BETA")

	mergeContext, err := zone.WithChildZone(parentContext, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	mergeInput := zonemerger.Input{
		SourceCorporaID: string(childPublished.CorporaID),
	}
	if err := service.ZoneMerger().Merge(mergeContext, mergeInput); err != nil {
		t.Fatal(err)
	}

	parentReceipt, err := service.Documents().SubmitDocument(parentContext, document.UploadCommand{
		Name: "parent.txt", MediaType: "text/plain",
		Content: bytes.NewBufferString("PARENT-GAMMA: Alpha works with Gamma."),
	})
	if err != nil {
		t.Fatal(err)
	}
	parentCorrelation := findDocumentCorrelation(t, service, parent.ID, parentReceipt.DocumentID)
	parentPublished := waitForZoneEvent[epochevents.PublishedV1](
		t,
		service,
		parent.ID,
		parentCorrelation,
	)

	parentCorpora, err := service.Documents().Corpora(
		parentContext,
		corpus.CorporaID(parentPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertChildCorporaReused(t, childCorpora, parentCorpora)
	if len(parentCorpora.Texts) != len(childCorpora.Texts)+1 {
		t.Fatalf(
			"Parent Corpora Text count = %d, want Child %d plus one Parent Text",
			len(parentCorpora.Texts),
			len(childCorpora.Texts),
		)
	}

	parentEntities, err := service.Knowledge().BrowseEntities(
		parentContext,
		queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	if parentEntities.EpochID != int64(parentPublished.EpochID) ||
		parentEntities.CorporaID != string(parentPublished.CorporaID) {
		t.Fatalf("Parent Knowledge publication = %#v, want Epoch event %#v", parentEntities.Publication, parentPublished)
	}
	assertEntityTitles(t, parentEntities.Items, "ALPHA", "BETA", "GAMMA")
	assertParentKnowledgeOwnsIDs(t, childEntities.Items, parentEntities.Items, "ALPHA", "BETA")

	if got := zoneEventCount[corpusevents.DocumentRecordedV1](t, service, parent.ID, parentCorrelation); got != 1 {
		t.Fatalf("Parent DocumentRecordedV1 count = %d, want only the local Parent event", got)
	}
	if got := zoneEventCount[corpusevents.TextPreparedV1](t, service, parent.ID, parentCorrelation); got != 1 {
		t.Fatalf("Parent TextPreparedV1 count = %d, want only the local Parent event", got)
	}
	if got := zoneEventCount[corpusevents.TextUnitsPreparedV1](t, service, parent.ID, parentCorrelation); got != 1 {
		t.Fatalf("Parent TextUnitsPreparedV1 count = %d, want only the local Parent event", got)
	}
	if got := zoneEventCount[corpusevents.CorporaPreparedV1](t, service, parent.ID, parentCorrelation); got != 1 {
		t.Fatalf("Parent CorporaPreparedV1 count = %d, want one final Corpus fact", got)
	}
	if got := zoneEventCount[epochevents.PublishedV1](t, service, parent.ID, parentCorrelation); got != 1 {
		t.Fatalf("Parent PublishedV1 count = %d, want one Epoch publication", got)
	}

	if err := service.ZoneMerger().Merge(mergeContext, mergeInput); err != nil {
		t.Fatalf("Merge() after Parent publication error = %v", err)
	}
	secondParentPublished := indexZoneDocument(
		t,
		service,
		parentContext,
		"zone-merge/parent-2",
		"parent-second.txt",
		"PARENT-GAMMA: Alpha works with Gamma in the next Epoch.",
		int64(parentPublished.EpochID),
	)
	secondParentCorpora, err := service.Documents().Corpora(
		parentContext,
		corpus.CorporaID(secondParentPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertChildCorporaReused(t, childCorpora, secondParentCorpora)
	if len(secondParentCorpora.Texts) != len(childCorpora.Texts)+2 {
		t.Fatalf(
			"second Parent Corpora Text count = %d, want Child %d plus two Parent Texts",
			len(secondParentCorpora.Texts),
			len(childCorpora.Texts),
		)
	}
	for _, childText := range childCorpora.Texts {
		if got := textOccurrences(secondParentCorpora, string(childText.TextID)); got != 1 {
			t.Fatalf("second Parent Corpora Child Text %q occurrences = %d, want 1", childText.TextID, got)
		}
	}
}

func TestZoneMergeConsumesPersistedBoundariesAfterServiceRestart(t *testing.T) {
	root := t.TempDir()
	models := newAssemblyModelServer(t)
	config := serviceConfigWithModelURL(t, root, models.URL)

	first, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Errorf("close first assembly Service: %v", err)
		}
	})
	firstRun := startZoneMergeDispatcher(t, first)
	parent, err := first.Zones().CreateRoot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, err := first.Zones().CreateChild(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	parentContext := bindZoneContext(t, parent.ID)
	childContext := bindZoneContext(t, child.ID)
	childPublished := indexZoneDocument(
		t,
		first,
		childContext,
		"zone-merge/restart-child",
		"child.txt",
		"CHILD-BETA: Alpha works with Beta.",
		0,
	)
	childCorpora, err := first.Documents().Corpora(
		childContext,
		corpus.CorporaID(childPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	mergeContext, err := zone.WithChildZone(parentContext, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ZoneMerger().Merge(mergeContext, zonemerger.Input{
		SourceCorporaID: string(childPublished.CorporaID),
	}); err != nil {
		t.Fatal(err)
	}
	firstRun.stop(t)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := assembly.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("close second assembly Service: %v", err)
		}
	})
	startZoneMergeDispatcher(t, second)
	parentContext = bindZoneContext(t, parent.ID)
	parentPublished := indexZoneDocument(
		t,
		second,
		parentContext,
		"zone-merge/restart-parent",
		"parent.txt",
		"PARENT-GAMMA: Alpha works with Gamma.",
		0,
	)
	parentCorpora, err := second.Documents().Corpora(
		parentContext,
		corpus.CorporaID(parentPublished.CorporaID),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertChildCorporaReused(t, childCorpora, parentCorpora)
	parentEntities, err := second.Knowledge().BrowseEntities(
		parentContext,
		queryknowledge.PageRequest{Limit: queryknowledge.MaximumPageSize},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEntityTitles(t, parentEntities.Items, "ALPHA", "BETA", "GAMMA")
}

type zoneMergeDispatcherRun struct {
	cancel  context.CancelFunc
	result  <-chan error
	stopped bool
}

func startZoneMergeDispatcher(t *testing.T, service *assembly.Service) *zoneMergeDispatcherRun {
	t.Helper()
	enableAutomaticControl(t, service)
	runContext, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- service.Run(runContext) }()
	run := &zoneMergeDispatcherRun{cancel: cancel, result: result}
	t.Cleanup(func() { run.stop(t) })
	return run
}

func enableAutomaticControl(t *testing.T, service *assembly.Service) {
	t.Helper()
	for _, action := range controlplane.Actions() {
		policy, err := service.ControlPolicies().Policy(t.Context(), action)
		if err == nil {
			if policy.Mode != controlplane.Automatic {
				t.Fatalf("%s Policy = %s, want Automatic", action, policy.Mode)
			}
			continue
		}
		if !errors.Is(err, controlplane.ErrPolicyNotFound) {
			t.Fatal(err)
		}
		if _, err := service.ControlPolicies().Publish(
			t.Context(),
			controlapplication.PublishPolicyInput{
				Action:         action,
				Mode:           controlplane.Automatic,
				MinimumPending: 1,
				MaximumWait:    time.Second,
				PublishedAt:    time.Now().UTC(),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
}

func (run *zoneMergeDispatcherRun) stop(t *testing.T) {
	t.Helper()
	if run == nil || run.stopped {
		return
	}
	run.stopped = true
	run.cancel()
	select {
	case err := <-run.result:
		if err != nil {
			t.Errorf("run Project Service: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Project Service did not stop after cancellation")
	}
}

func indexZoneDocument(
	t *testing.T,
	service *assembly.Service,
	ctx context.Context,
	correlation string,
	name string,
	content string,
	expectedEpoch int64,
) epochevents.PublishedV1 {
	t.Helper()
	receipt, err := service.Documents().SubmitDocument(ctx, document.UploadCommand{
		Name: name, MediaType: "text/plain",
		Content: bytes.NewBufferString(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	documentCorrelation := findDocumentCorrelation(t, service, zoneID, receipt.DocumentID)
	published := waitForZoneEvent[epochevents.PublishedV1](
		t,
		service,
		zoneID,
		documentCorrelation,
	)
	if int64(published.EpochID) != expectedEpoch+1 {
		t.Fatalf("published Epoch = %d, want %d", published.EpochID, expectedEpoch+1)
	}
	_ = correlation
	return published
}

func findDocumentCorrelation(
	t *testing.T,
	service *assembly.Service,
	zoneID zone.ID,
	documentID document.ID,
) journal.CorrelationID {
	t.Helper()
	events, err := service.Journal().Entries(bindZoneContext(t, zoneID), 0, 512)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.ZoneID != zoneID ||
			event.Type != journal.EventKeyFor[corpusevents.DocumentRecordedV1]().Type {
			continue
		}
		body, err := journal.DecodeJSONBody[corpusevents.DocumentRecordedV1](event)
		if err != nil {
			t.Fatal(err)
		}
		if body.DocumentID == corpusevents.DocumentID(documentID) {
			return event.CorrelationID
		}
	}
	t.Fatalf("Document %q record event was not found", documentID)
	return ""
}

func waitForZoneEvent[T journal.EventBody](
	t *testing.T,
	service *assembly.Service,
	zoneID zone.ID,
	correlationID journal.CorrelationID,
) T {
	t.Helper()
	key := journal.EventKeyFor[T]()
	deadline := time.Now().Add(zoneMergeIntegrationWait)
	for {
		events, err := service.Journal().Entries(bindZoneContext(t, zoneID), 0, 512)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.ZoneID != zoneID || event.CorrelationID != correlationID ||
				event.Type != key.Type || event.SchemaVersion != key.Version {
				continue
			}
			body, err := journal.DecodeJSONBody[T](event)
			if err != nil {
				t.Fatal(err)
			}
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"Journal event %s v%d for Zone %q Correlation %q was not published",
				key.Type,
				key.Version,
				zoneID,
				correlationID,
			)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func zoneEventCount[T journal.EventBody](
	t *testing.T,
	service *assembly.Service,
	zoneID zone.ID,
	correlationID journal.CorrelationID,
) int {
	t.Helper()
	key := journal.EventKeyFor[T]()
	events, err := service.Journal().Entries(bindZoneContext(t, zoneID), 0, 512)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.ZoneID == zoneID && event.CorrelationID == correlationID &&
			event.Type == key.Type && event.SchemaVersion == key.Version {
			count++
		}
	}
	return count
}

func assertChildCorporaReused(t *testing.T, child, parent corpus.Corpora) {
	t.Helper()
	parentTexts := make(map[string]corpus.ChunkedText, len(parent.Texts))
	for _, value := range parent.Texts {
		parentTexts[string(value.TextID)] = value
	}
	for _, value := range child.Texts {
		reused, exists := parentTexts[string(value.TextID)]
		if !exists {
			t.Fatalf("Parent Corpora does not contain Child Text %q", value.TextID)
		}
		if !reflect.DeepEqual(reused, value) {
			t.Fatalf("Parent Child Text %q = %#v, want exact reused value %#v", value.TextID, reused, value)
		}
	}
}

func textOccurrences(value corpus.Corpora, textID string) int {
	count := 0
	for _, text := range value.Texts {
		if string(text.TextID) == textID {
			count++
		}
	}
	return count
}

func assertEntityTitles(t *testing.T, entities []queryknowledge.Entity, titles ...string) {
	t.Helper()
	found := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		found[entity.Title] = struct{}{}
	}
	for _, title := range titles {
		if _, exists := found[title]; !exists {
			t.Fatalf("Knowledge Entity titles = %v, want %q", found, title)
		}
	}
}

func assertParentKnowledgeOwnsIDs(
	t *testing.T,
	child []queryknowledge.Entity,
	parent []queryknowledge.Entity,
	titles ...string,
) {
	t.Helper()
	childIDs := make(map[string]string, len(child))
	for _, entity := range child {
		childIDs[entity.Title] = entity.ID
	}
	parentIDs := make(map[string]string, len(parent))
	for _, entity := range parent {
		parentIDs[entity.Title] = entity.ID
	}
	for _, title := range titles {
		if childIDs[title] == "" || parentIDs[title] == "" {
			t.Fatalf("Knowledge IDs for %q are incomplete: Child=%q Parent=%q", title, childIDs[title], parentIDs[title])
		}
		if childIDs[title] == parentIDs[title] {
			t.Fatalf("Parent reused Child Knowledge Entity ID %q for %q", parentIDs[title], title)
		}
	}
}

func newAssemblyModelServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/chat/completions":
			body, err := io.ReadAll(request.Body)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			var input struct {
				ResponseFormat struct {
					JSONSchema struct {
						Name string `json:"name"`
					} `json:"json_schema"`
				} `json:"response_format"`
			}
			if err := json.Unmarshal(body, &input); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			content := `<|COMPLETE|>`
			switch input.ResponseFormat.JSONSchema.Name {
			case "community_report":
				content = `{"title":"Alpha","summary":"Alpha summary","findings":[{"summary":"Alpha finding","explanation":"Alpha evidence"}],"rating":5,"rating_explanation":"Alpha rating"}`
			case "report_fragment":
				content = `{"content":"Alpha evidence"}`
			case "description_summary":
				content = `{"description":"Alpha description"}`
			case "knowledge_graph":
				if strings.Contains(string(body), "CHILD-DELTA") {
					content = `{"entities":[{"name":"ALPHA","type":"PERSON","aliases":[],"description":"Alpha description"},{"name":"DELTA","type":"PERSON","aliases":[],"description":"Delta description"}],"relations":[{"source":"ALPHA","target":"DELTA","type":"COLLABORATION","description":"Alpha works with Delta","weight":1}]}`
				} else if strings.Contains(string(body), "PARENT-GAMMA") {
					content = `{"entities":[{"name":"ALPHA","type":"PERSON","aliases":[],"description":"Alpha description"},{"name":"GAMMA","type":"PERSON","aliases":[],"description":"Gamma description"}],"relations":[{"source":"ALPHA","target":"GAMMA","type":"COLLABORATION","description":"Alpha works with Gamma","weight":1}]}`
				} else {
					content = `{"entities":[{"name":"ALPHA","type":"PERSON","aliases":[],"description":"Alpha description"},{"name":"BETA","type":"PERSON","aliases":[],"description":"Beta description"}],"relations":[{"source":"ALPHA","target":"BETA","type":"COLLABORATION","description":"Alpha works with Beta","weight":1}]}`
				}
			default:
				if strings.Contains(strings.ToLower(string(body)), "claim description:") {
					content = `(ALPHA<|>BETA<|>COLLABORATION<|>TRUE<|>NONE<|>NONE<|>Alpha works with Beta<|>Alpha works with Beta)<|COMPLETE|>`
				}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id": "chatcmpl-zone-merge", "object": "chat.completion", "created": 1,
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
			data := make([]map[string]any, len(texts))
			for index := range texts {
				data[index] = map[string]any{
					"object": "embedding", "index": index, "embedding": []float64{1, 0},
				}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"object": "list", "model": "embedding-test", "data": data,
				"usage": map[string]any{"prompt_tokens": 1, "total_tokens": 1},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
