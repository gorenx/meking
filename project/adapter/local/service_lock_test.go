package local

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectServiceLockRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	const original = "unchanged"
	if err := os.WriteFile(target, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, projectServiceLockFile)); err != nil {
		t.Skipf("create lock symlink: %v", err)
	}

	lock, err := AcquireProjectServiceLock(root)
	if err == nil {
		_ = lock.Close()
		t.Fatal("AcquireProjectServiceLock accepted a symlink")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("lock symlink target = %q, want %q", content, original)
	}
}
