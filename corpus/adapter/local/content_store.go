package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/memoria-space/meking/corpus/document"
)

type ContentStore struct {
	root         string
	staging      string
	publications sync.Mutex
}

var _ document.ContentStore = (*ContentStore)(nil)

func NewContentStore(root string) (*ContentStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("create local document ContentStore: root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local document content root: %w", err)
	}
	if err = os.MkdirAll(absolute, 0o755); err != nil {
		return nil, fmt.Errorf("create local document content root: %w", err)
	}
	staging := filepath.Join(absolute, ".staging")
	if err = os.MkdirAll(staging, 0o700); err != nil {
		return nil, fmt.Errorf("create local document staging directory: %w", err)
	}
	return &ContentStore{root: filepath.Clean(absolute), staging: staging}, nil
}

func (s *ContentStore) Stage(ctx context.Context) (document.StagedContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(s.staging, "document-*")
	if err != nil {
		return nil, fmt.Errorf("create staged local document content: %w", err)
	}
	return &stagedContent{store: s, file: file, path: file.Name()}, nil
}

type stagedContent struct {
	store              *ContentStore
	file               *os.File
	path               string
	publishedPath      string
	releasePublication func()
	published          bool
	created            bool
	confirmed          bool
}

func (s *stagedContent) Write(content []byte) (int, error) {
	if s == nil || s.file == nil || s.published {
		return 0, errors.New("write staged local document content: stage is closed")
	}
	return s.file.Write(content)
}

func (s *stagedContent) Publish(
	ctx context.Context,
	value document.Document,
) (document.Location, error) {
	if s == nil || s.file == nil || s.published {
		return "", errors.New("commit staged local document content: stage is closed")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := document.Restore(value); err != nil {
		return "", err
	}
	if err := s.file.Sync(); err != nil {
		return "", fmt.Errorf("sync local document content: %w", err)
	}
	if err := s.file.Chmod(0o644); err != nil {
		return "", fmt.Errorf("set local document content permissions: %w", err)
	}
	if err := s.file.Close(); err != nil {
		return "", fmt.Errorf("close local document content: %w", err)
	}
	s.file = nil
	encoded := value.Digest
	directory := filepath.Join(s.store.root, encoded[:2])
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create local document content directory: %w", err)
	}
	relative := filepath.Join(encoded[:2], encoded+safeExtension(value.Name))
	path := filepath.Join(s.store.root, relative)
	s.store.publications.Lock()
	s.releasePublication = s.store.publications.Unlock
	if err := os.Link(s.path, path); errors.Is(err, os.ErrExist) {
		if err := verifyFile(path, value); err != nil {
			return "", s.failPublication(err)
		}
	} else if err != nil {
		return "", s.failPublication(fmt.Errorf("publish local document content: %w", err))
	} else {
		s.created = true
		s.publishedPath = path
	}
	if err := os.Remove(s.path); err != nil {
		return "", s.failPublication(fmt.Errorf("remove staged local document content: %w", err))
	}
	s.path = ""
	s.published = true
	return document.Location(filepath.ToSlash(relative)), nil
}

func (s *stagedContent) Confirm() {
	if s != nil && s.published && !s.confirmed {
		s.confirmed = true
		s.publishedPath = ""
		s.release()
	}
}

func (s *stagedContent) Discard() error {
	if s == nil || s.confirmed {
		return nil
	}
	var closeErr, stagedRemoveErr, publishedRemoveErr error
	if s.file != nil {
		closeErr = s.file.Close()
		s.file = nil
	}
	if s.path != "" {
		stagedRemoveErr = os.Remove(s.path)
		if errors.Is(stagedRemoveErr, os.ErrNotExist) {
			stagedRemoveErr = nil
		}
		s.path = ""
	}
	if s.created && s.publishedPath != "" {
		publishedRemoveErr = os.Remove(s.publishedPath)
		if errors.Is(publishedRemoveErr, os.ErrNotExist) {
			publishedRemoveErr = nil
		}
		s.publishedPath = ""
	}
	s.release()
	if err := errors.Join(closeErr, stagedRemoveErr, publishedRemoveErr); err != nil {
		return contentCleanupError{cause: err}
	}
	return nil
}

func (s *stagedContent) failPublication(failure error) error {
	return errors.Join(failure, s.Discard())
}

func (s *stagedContent) release() {
	if s.releasePublication != nil {
		s.releasePublication()
		s.releasePublication = nil
	}
}

type contentCleanupError struct{ cause error }

func (contentCleanupError) Error() string     { return "discard local document content" }
func (err contentCleanupError) Unwrap() error { return err.cause }

func safeExtension(name string) string {
	extension := strings.ToLower(filepath.Ext(filepath.Base(name)))
	if len(extension) < 2 || len(extension) > 17 {
		return ""
	}
	for _, character := range extension[1:] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return ""
		}
	}
	return extension
}

func (s *ContentStore) Open(
	ctx context.Context,
	location document.Location,
) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, owned := s.resolve(location)
	if !owned {
		return nil, document.ErrNotFound
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, document.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open local document content: %w", err)
	}
	return file, nil
}

func (s *ContentStore) resolve(location document.Location) (string, bool) {
	value := string(location)
	if value == "" || filepath.IsAbs(value) {
		return "", false
	}
	path := filepath.Join(s.root, filepath.FromSlash(value))
	path = filepath.Clean(path)
	relative, err := filepath.Rel(s.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

func (s *ContentStore) location(path string) (document.Location, bool) {
	relative, err := filepath.Rel(s.root, filepath.Clean(path))
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return document.Location(filepath.ToSlash(relative)), true
}

func verifyFile(path string, value document.Document) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open existing local document content: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return fmt.Errorf("read existing local document content: %w", err)
	}
	if size != value.Size || hex.EncodeToString(hash.Sum(nil)) != value.Digest {
		return fmt.Errorf("%w: existing local content does not match Document", document.ErrStorageIntegrity)
	}
	return nil
}
