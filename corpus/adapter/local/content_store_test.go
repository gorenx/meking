package local

import (
	"strings"
	"testing"
	"time"

	"github.com/memoria-space/meking/corpus/document"
)

func TestContentStorePreservesSafeExtensionForReconciliation(t *testing.T) {
	store, err := NewContentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value, err := document.New("source.txt", "text/plain", []byte("knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()
	if _, err := staged.Write([]byte("knowledge")); err != nil {
		t.Fatal(err)
	}
	location, err := staged.Publish(t.Context(), value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(location), ".txt") {
		t.Fatalf("Location = %q, want preserved .txt extension", location)
	}
	staged.Confirm()
}

func TestContentStoreSerializesPublishedContentUntilConfirmation(t *testing.T) {
	store, err := NewContentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value, err := document.New("source.txt", "text/plain", []byte("knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	first := stageDocumentContent(t, store)
	second := stageDocumentContent(t, store)
	if _, err := first.Publish(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	type publication struct {
		location document.Location
		err      error
	}
	started := make(chan struct{})
	completed := make(chan publication, 1)
	go func() {
		close(started)
		location, publishErr := second.Publish(t.Context(), value)
		completed <- publication{location: location, err: publishErr}
	}()
	<-started
	select {
	case result := <-completed:
		t.Fatalf("concurrent Publish() completed before compensation: %#v", result)
	case <-time.After(25 * time.Millisecond):
	}
	if err := first.Discard(); err != nil {
		t.Fatal(err)
	}
	result := <-completed
	if result.err != nil {
		t.Fatal(result.err)
	}
	second.Confirm()
	opened, err := store.Open(t.Context(), result.location)
	if err != nil {
		t.Fatalf("Open() after replacement publication error = %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
}

func stageDocumentContent(t *testing.T, store *ContentStore) document.StagedContent {
	t.Helper()
	staged, err := store.Stage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := staged.Write([]byte("knowledge")); err != nil {
		t.Fatal(err)
	}
	return staged
}
