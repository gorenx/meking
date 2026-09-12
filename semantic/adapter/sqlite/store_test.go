package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/memoria-space/meking/semantic"
	semanticsqlite "github.com/memoria-space/meking/semantic/adapter/sqlite"
)

func TestNamespaceAddMakesAtomicBatchReadable(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	if err := store.Add(ctx, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
		{ID: "entity/2@3", Values: []float64{0, 1}},
	}); err != nil {
		t.Fatalf("add vectors: %v", err)
	}

	reader, err := store.Open(ctx, "knowledge/12")
	if err != nil {
		t.Fatalf("open namespace: %v", err)
	}
	defer reader.Close()
	_, err = reader.Info(ctx)
	if err != nil {
		t.Fatalf("read namespace info: %v", err)
	}

	page, err := reader.IDs(ctx, "", 10)
	if err != nil {
		t.Fatalf("read namespace IDs: %v", err)
	}
	if want := []string{"entity/1@1", "entity/2@3"}; !reflect.DeepEqual(page.Items, want) {
		t.Fatalf("namespace IDs: got %v, want %v", page.Items, want)
	}
}

func TestNamespaceAddIsIdempotentAcrossBatches(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	if err := store.Add(ctx, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
	}); err != nil {
		t.Fatalf("add first batch: %v", err)
	}
	if err := store.Add(ctx, "knowledge/12", []semantic.Vector{
		{ID: "entity/2@1", Values: []float64{0, 1}},
		{ID: "entity/1@1", Values: []float64{1, 0}},
	}); err != nil {
		t.Fatalf("add overlapping batch: %v", err)
	}
	reader, err := store.Open(ctx, "knowledge/12")
	if err != nil {
		t.Fatalf("open namespace: %v", err)
	}
	defer reader.Close()
	_, err = reader.Info(ctx)
	if err != nil {
		t.Fatalf("read namespace info: %v", err)
	}

}

func TestNamespaceReaderPaginatesAndFilters(t *testing.T) {
	t.Parallel()

	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	addNamespace(t, testContext(t), store, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
		{ID: "entity/2@1", Values: []float64{0, 1}},
		{ID: "entity/3@1", Values: []float64{1, 1}},
	})
	reader, err := store.Open(testContext(t), "knowledge/12")
	if err != nil {
		t.Fatalf("open namespace: %v", err)
	}
	defer reader.Close()
	first, err := reader.IDs(testContext(t), "", 2)
	if err != nil {
		t.Fatalf("read first ID page: %v", err)
	}
	if want := []string{"entity/1@1", "entity/2@1"}; !reflect.DeepEqual(first.Items, want) || !first.HasMore || first.NextAfter != want[1] {
		t.Fatalf("first ID page = %+v, want items %v with more", first, want)
	}
	second, err := reader.IDs(testContext(t), first.NextAfter, 2)
	if err != nil {
		t.Fatalf("read second ID page: %v", err)
	}
	if want := []string{"entity/3@1"}; !reflect.DeepEqual(second.Items, want) || second.HasMore || second.NextAfter != want[0] {
		t.Fatalf("second ID page = %+v, want items %v without more", second, want)
	}
	matches, err := reader.Search(
		testContext(t), []float64{1, 0}, 2,
		map[string]string{"zone_id": "10000000-0000-4000-8000-000000000001"},
	)
	if err != nil {
		t.Fatalf("search scoped namespace: %v", err)
	}
	if len(matches) != 2 || matches[0].ID != "entity/1@1" || matches[1].ID != "entity/3@1" {
		t.Fatalf("scoped matches = %+v", matches)
	}
}

func TestConcurrentAddsReuseCanonicalVector(t *testing.T) {
	t.Parallel()

	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	start := make(chan struct{})
	errorsByNamespace := make(chan error, 2)
	for _, namespace := range []semantic.Namespace{"knowledge/12", "knowledge/13"} {
		namespace := namespace
		go func() {
			<-start
			errorsByNamespace <- store.Add(testContext(t), namespace, []semantic.Vector{
				{ID: "entity/1@1", Values: []float64{1, 0}},
			})
		}()
	}
	close(start)
	for range 2 {
		if err := <-errorsByNamespace; err != nil {
			t.Fatalf("concurrent Add() error = %v", err)
		}
	}
	for _, namespace := range []semantic.Namespace{"knowledge/12", "knowledge/13"} {
		reader, err := store.Open(testContext(t), namespace)
		if err != nil {
			t.Fatalf("open namespace %q: %v", namespace, err)
		}
		info, infoErr := reader.Info(testContext(t))
		closeErr := reader.Close()
		if err := errors.Join(infoErr, closeErr); err != nil {
			t.Fatalf("read namespace %q: %v", namespace, err)
		}
		if info.Dimension != 2 {
			t.Fatalf("namespace %q info = %+v", namespace, info)
		}
	}
}

func TestVectorCacheLookupBatchesSQLiteParameters(t *testing.T) {
	t.Parallel()

	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	ids := make([]string, 32_767)
	for index := range ids {
		ids[index] = fmt.Sprintf("vector/%05d", index)
	}
	vectors, err := store.Lookup(testContext(t), ids)
	if err != nil {
		t.Fatalf("lookup vectors beyond one SQLite parameter set: %v", err)
	}
	if len(vectors) != 0 {
		t.Fatalf("lookup absent vectors returned %d values", len(vectors))
	}
}

func TestNewStoreRejectsUnexpectedStaticSchema(t *testing.T) {
	t.Parallel()

	database := openDatabase(t, filepath.Join(t.TempDir(), "semantic.db"))
	if _, err := database.Exec(`
		CREATE TABLE vector_database (singleton INTEGER PRIMARY KEY, model TEXT, dimension INTEGER);
		CREATE TABLE vectors (id INTEGER PRIMARY KEY);
		CREATE TABLE namespace_config (id INTEGER PRIMARY KEY);
		CREATE TABLE namespace_vectors (id INTEGER PRIMARY KEY);
		PRAGMA user_version = 7;
	`); err != nil {
		t.Fatalf("create incompatible schema: %v", err)
	}
	if _, err := semanticsqlite.NewStore(
		database, "embedding-test", slog.New(slog.DiscardHandler),
	); err == nil {
		t.Fatal("NewStore() accepted an incompatible version 7 schema")
	}
}

func TestNewStoreRejectsMissingConfiguredVectorTable(t *testing.T) {
	t.Parallel()

	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	addNamespace(t, testContext(t), store, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
	})
	if _, err := database.Exec(`DROP TABLE "vector_values_2"`); err != nil {
		t.Fatalf("drop configured vec0 table: %v", err)
	}
	if _, err := semanticsqlite.NewStore(
		database, "embedding-test", slog.New(slog.DiscardHandler),
	); err == nil {
		t.Fatal("NewStore() accepted a missing configured vec0 table")
	}
}

func TestNewStoreRejectsEmbeddingModelMismatch(t *testing.T) {
	t.Parallel()

	_, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	if _, err := semanticsqlite.NewStore(
		database, "other-model", slog.New(slog.DiscardHandler),
	); err == nil {
		t.Fatal("NewStore() accepted a different embedding model")
	}
}

func TestFailedAddRollsBackNamespaceMetadata(t *testing.T) {
	t.Parallel()

	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	addNamespace(t, testContext(t), store, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
	})
	if _, err := database.Exec(`
		DROP TABLE "vector_values_2";
		CREATE TABLE "vector_values_2" (id INTEGER PRIMARY KEY);
	`); err != nil {
		t.Fatalf("replace vec0 table: %v", err)
	}
	if err := store.Add(testContext(t), "knowledge/13", []semantic.Vector{
		{ID: "entity/2@1", Values: []float64{0, 1}},
	}); err == nil {
		t.Fatal("Add() accepted an incompatible vec0 table")
	}
	if _, err := store.Open(testContext(t), "knowledge/13"); !errors.Is(err, semantic.ErrNamespaceNotFound) {
		t.Fatalf("open failed Add Namespace: got %v, want ErrNamespaceNotFound", err)
	}
}

func TestOpenRejectsMismatchedMemberAndIndexedVector(t *testing.T) {
	t.Parallel()

	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	addNamespace(t, testContext(t), store, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
	})
	addNamespace(t, testContext(t), store, "knowledge/13", []semantic.Vector{
		{ID: "entity/2@1", Values: []float64{0, 1}},
	})
	if _, err := database.Exec(`
		UPDATE namespace_vectors
		SET vector_row_id = (
			SELECT id FROM vectors
			WHERE vector_id = 'entity/2@1'
		)
		WHERE zone_id = '10000000-0000-4000-8000-000000000001'
		  AND namespace_row_id = (
			SELECT id FROM namespace_config
			WHERE zone_id = '10000000-0000-4000-8000-000000000001'
			  AND name = 'knowledge/12'
		)
	`); err != nil {
		t.Fatalf("corrupt Namespace membership: %v", err)
	}
	if _, err := store.Open(testContext(t), "knowledge/12"); err == nil {
		t.Fatal("Open() accepted mismatched membership and vec0 identities")
	}
}

func TestDeleteRemovesIndexedRowsIndependentlyOfStoredCount(t *testing.T) {
	t.Parallel()

	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	addNamespace(t, testContext(t), store, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
	})
	addNamespace(t, testContext(t), store, "knowledge/empty", nil)
	var namespaceRowID int64
	if err := database.QueryRow(
		`SELECT id FROM namespace_config
		 WHERE zone_id = '10000000-0000-4000-8000-000000000001'
		   AND name = 'knowledge/empty'`,
	).Scan(&namespaceRowID); err != nil {
		t.Fatalf("read empty Namespace row ID: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO "vector_values_2" (rowid, embedding, namespace_row_id, vector_id)
		VALUES (9999, vec_f32('[1, 0]'), ?, 'orphan')
	`, namespaceRowID); err != nil {
		t.Fatalf("insert stale vec0 row: %v", err)
	}
	if err := store.Delete(testContext(t), "knowledge/empty"); err != nil {
		t.Fatalf("delete empty Namespace: %v", err)
	}
	var indexed int
	if err := database.QueryRow(
		`SELECT count(*) FROM "vector_values_2" WHERE namespace_row_id = ?`, namespaceRowID,
	).Scan(&indexed); err != nil {
		t.Fatalf("count stale vec0 rows: %v", err)
	}
	if indexed != 0 {
		t.Fatalf("stale vec0 rows after Delete = %d", indexed)
	}
}

func TestVectorCacheReusesCanonicalVectorAcrossNamespaces(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	addNamespace(t, ctx, store, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
	})
	model := &embeddingStub{byText: map[string][]float64{
		"new entity": {0, 1},
	}}
	service, err := semantic.NewService(
		model,
		tokenCounterStub{},
		store,
		semantic.DefaultGenerationConfig(),
	)
	if err != nil {
		t.Fatalf("create Semantic Service: %v", err)
	}
	if err := service.Add(ctx, "knowledge/13", []semantic.Input{
		{ID: "entity/1@1", Text: "existing entity"},
		{ID: "entity/2@1", Text: "new entity"},
	}); err != nil {
		t.Fatalf("add second namespace vectors: %v", err)
	}
	if want := [][]string{{"new entity"}}; !reflect.DeepEqual(model.Calls(), want) {
		t.Fatalf("embedding calls: got %v, want %v", model.Calls(), want)
	}

	reader, err := store.Open(ctx, "knowledge/13")
	if err != nil {
		t.Fatalf("open second namespace: %v", err)
	}
	defer reader.Close()
	matches, err := reader.Search(ctx, []float64{1, 0}, 2, nil)
	if err != nil {
		t.Fatalf("search second namespace: %v", err)
	}
	if len(matches) != 2 || matches[0].ID != "entity/1@1" || matches[0].Score != 1 {
		t.Fatalf("search canonical vector: got %+v", matches)
	}
}

type embeddingStub struct {
	mu     sync.Mutex
	byText map[string][]float64
	calls  [][]string
}

func (s *embeddingStub) Embed(_ context.Context, texts []string) (semantic.EmbeddingBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request := append([]string(nil), texts...)
	s.calls = append(s.calls, request)
	vectors := make([][]float64, len(texts))
	for index, text := range texts {
		vectors[index] = append([]float64(nil), s.byText[text]...)
	}
	return semantic.EmbeddingBatch{Vectors: vectors}, nil
}

func (s *embeddingStub) Calls() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([][]string, len(s.calls))
	for index, call := range s.calls {
		result[index] = append([]string(nil), call...)
	}
	return result
}

type tokenCounterStub struct{}

func (tokenCounterStub) Count(text string) (int, error) {
	return len(text), nil
}

func TestOpenReaderRemainsStableAfterNamespaceDeletion(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	store, database := openStore(t, filepath.Join(t.TempDir(), "semantic.db"))
	defer database.Close()
	addNamespace(t, ctx, store, "knowledge/12", []semantic.Vector{
		{ID: "entity/1@1", Values: []float64{1, 0}},
	})
	reader, err := store.Open(ctx, "knowledge/12")
	if err != nil {
		t.Fatalf("open namespace: %v", err)
	}
	defer reader.Close()
	if err := store.Delete(ctx, "knowledge/12"); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}
	if _, err := store.Open(ctx, "knowledge/12"); !errors.Is(err, semantic.ErrNamespaceNotFound) {
		t.Fatalf("open deleted namespace: got %v, want ErrNamespaceNotFound", err)
	}
	matches, err := reader.Search(ctx, []float64{1, 0}, 1, nil)
	if err != nil {
		t.Fatalf("search pinned reader: %v", err)
	}
	if len(matches) != 1 || matches[0].ID != "entity/1@1" {
		t.Fatalf("pinned reader matches: got %+v", matches)
	}
}

func addNamespace(t *testing.T, ctx context.Context, store semantic.NamespaceStore, namespace semantic.Namespace, vectors []semantic.Vector) {
	t.Helper()
	if err := store.Add(ctx, namespace, vectors); err != nil {
		t.Fatalf("add namespace %q: %v", namespace, err)
	}
}
