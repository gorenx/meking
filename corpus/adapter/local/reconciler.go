package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
)

const reconcilePageSize = 500

type DocumentService interface {
	RegisterDocument(context.Context, corpus.DocumentRegistration) (corpus.RegisteredDocument, error)
	LocatedDocument(context.Context, document.Location) (document.LocatedDocument, error)
	ListLocatedDocuments(context.Context, document.Page) (document.LocatedDocumentPage, error)
}

type ChangedFile struct {
	DocumentID     document.ID
	Location       document.Location
	ExpectedDigest string
	ActualDigest   string
}

type MissingFile struct {
	DocumentID document.ID
	Location   document.Location
}

type FileFailure struct {
	Location document.Location
	Err      error
}

type DuplicateFile struct {
	DocumentID document.ID
	Location   document.Location
}

type ReconcileResult struct {
	DocumentsAdded []document.ID
	Duplicates     []DuplicateFile
	Changed        []ChangedFile
	Missing        []MissingFile
	Failures       []FileFailure
}

type Reconciler struct {
	content   *ContentStore
	documents DocumentService
	files     *regexp.Regexp
}

func NewReconciler(
	content *ContentStore,
	documents DocumentService,
	filePattern string,
) (*Reconciler, error) {
	if content == nil || documents == nil {
		return nil, fmt.Errorf("create local Document Reconciler: dependencies are required")
	}
	filePattern = strings.TrimSpace(filePattern)
	if filePattern == "" {
		filePattern = ".*"
	}
	files, err := regexp.Compile(filePattern)
	if err != nil {
		return nil, fmt.Errorf("create local Document Reconciler: invalid file pattern: %w", err)
	}
	return &Reconciler{content: content, documents: documents, files: files}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context) (ReconcileResult, error) {
	result := ReconcileResult{}
	err := filepath.WalkDir(r.content.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr == nil && entry.IsDir() && path == r.content.staging {
			return filepath.SkipDir
		}
		location, owned := r.content.location(path)
		if walkErr != nil {
			if path == r.content.root {
				return walkErr
			}
			result.Failures = append(result.Failures, FileFailure{Location: location, Err: walkErr})
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() || !owned {
			return nil
		}
		path = filepath.Clean(path)
		existing, lookupErr := r.documents.LocatedDocument(ctx, location)
		if lookupErr == nil {
			digest, digestErr := fileDigest(path)
			if digestErr != nil {
				result.Failures = append(result.Failures, FileFailure{Location: location, Err: digestErr})
				return nil
			}
			if digest != existing.Document.Digest {
				result.Changed = append(result.Changed,
					ChangedFile{
						DocumentID: existing.Document.ID, Location: location,
						ExpectedDigest: existing.Document.Digest, ActualDigest: digest,
					},
				)
			}
			return nil
		}
		if !errors.Is(lookupErr, document.ErrNotFound) {
			result.Failures = append(result.Failures, FileFailure{Location: location, Err: lookupErr})
			return nil
		}
		if !r.files.MatchString(string(location)) {
			return nil
		}
		mediaType := "application/octet-stream"
		if detected := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); detected != "" {
			if parsed, _, parseErr := mime.ParseMediaType(detected); parseErr == nil {
				mediaType = parsed
			}
		}
		registeredFile, addErr := r.documents.RegisterDocument(ctx, corpus.DocumentRegistration{
			Name:      entry.Name(),
			MediaType: mediaType,
			Location:  location,
		})
		if addErr != nil {
			result.Failures = append(result.Failures, FileFailure{Location: location, Err: addErr})
			return nil
		}
		if registeredFile.DocumentCreated {
			result.DocumentsAdded = append(result.DocumentsAdded, registeredFile.DocumentID)
		}
		if !registeredFile.DocumentCreated {
			result.Duplicates = append(result.Duplicates, DuplicateFile{
				DocumentID: registeredFile.DocumentID, Location: location,
			})
		}
		return nil
	})
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("reconcile local Documents: %w", err)
	}
	if err := r.appendMissing(ctx, &result); err != nil {
		return ReconcileResult{}, err
	}
	sortReconcileResult(&result)
	return result, nil
}

func (r *Reconciler) appendMissing(ctx context.Context, result *ReconcileResult) error {
	page := document.Page{Limit: reconcilePageSize}
	for {
		values, err := r.documents.ListLocatedDocuments(ctx, page)
		if err != nil {
			return err
		}
		for _, value := range values.Documents {
			path, owned := r.content.resolve(value.Location)
			if !owned {
				continue
			}
			info, statErr := os.Stat(path)
			switch {
			case errors.Is(statErr, os.ErrNotExist):
				result.Missing = append(result.Missing, MissingFile{
					DocumentID: value.Document.ID, Location: value.Location,
				})
			case statErr != nil:
				result.Failures = append(result.Failures, FileFailure{Location: value.Location, Err: statErr})
			case !info.Mode().IsRegular():
				result.Missing = append(result.Missing, MissingFile{
					DocumentID: value.Document.ID, Location: value.Location,
				})
			}
		}
		if values.Next == nil {
			return nil
		}
		page = *values.Next
	}
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sortReconcileResult(result *ReconcileResult) {
	sort.Slice(result.DocumentsAdded, func(i, j int) bool { return result.DocumentsAdded[i] < result.DocumentsAdded[j] })
	sort.Slice(result.Duplicates, func(i, j int) bool { return result.Duplicates[i].Location < result.Duplicates[j].Location })
	sort.Slice(result.Changed, func(i, j int) bool { return result.Changed[i].Location < result.Changed[j].Location })
	sort.Slice(result.Missing, func(i, j int) bool { return result.Missing[i].Location < result.Missing[j].Location })
	sort.Slice(result.Failures, func(i, j int) bool { return result.Failures[i].Location < result.Failures[j].Location })
}
