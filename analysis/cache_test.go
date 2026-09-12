package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateCacheKeySeparatesAnalysisIdentityWithoutRawInput(t *testing.T) {
	t.Parallel()
	input := CacheKeyInput{
		Capability: CapabilitySentences, ContractVersion: ContractVersion,
		ConfigurationHash: "english", InputHash: "document-sha",
	}
	first, err := CreateCacheKey(input)
	if err != nil {
		t.Fatalf("CreateCacheKey() error = %v", err)
	}
	second, err := CreateCacheKey(input)
	if err != nil || second != first {
		t.Fatalf("second key = %q, %v", second, err)
	}
	input.ConfigurationHash = "spanish"
	third, err := CreateCacheKey(input)
	if err != nil || third == first {
		t.Fatalf("third key = %q, %v", third, err)
	}
	if bytes.Contains([]byte(first), []byte("document-sha")) {
		t.Fatalf("cache key exposes input identity: %q", first)
	}
}

func TestSQLiteCachePersistsCompleteEntriesPrivately(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cache", "analysis-cache.sqlite")
	cache, err := OpenSQLiteCache(path)
	if err != nil {
		t.Fatalf("OpenSQLiteCache() error = %v", err)
	}
	entry := CacheEntry{
		Capability: CapabilitySentences,
		Payload:    []byte(`{"spans":[]}`), CreatedAt: time.Now().UTC(),
	}
	if err := cache.Put(t.Context(), "key", entry); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	entry.Payload[0] = 'x'
	if err := cache.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %o", info.Mode().Perm())
	}
	cache, err = OpenSQLiteCache(path)
	if err != nil {
		t.Fatalf("reopen cache error = %v", err)
	}
	defer cache.Close()
	got, ok, err := cache.Get(t.Context(), "key")
	if err != nil || !ok {
		t.Fatalf("Get() = %#v, %t, %v", got, ok, err)
	}
	if string(got.Payload) != `{"spans":[]}` {
		t.Fatalf("Get() = %#v", got)
	}
	got.Payload[0] = 'x'
	again, ok, err := cache.Get(t.Context(), "key")
	if err != nil || !ok || string(again.Payload) != `{"spans":[]}` {
		t.Fatalf("second Get() = %#v, %t, %v", again, ok, err)
	}
}
