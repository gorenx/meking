// Package vectorindex owns the text and exact Entity Version membership of an
// Entity vector Namespace.
package vectorindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

import "github.com/memoria-space/meking/knowledge"

const PageSize = 256

var ErrNotFound = errors.New("Entity vector Namespace not found")

type Source struct {
	ID   string
	Text string
}

type Page struct {
	Items   []Source
	HasMore bool
}

type Knowledge interface {
	OpenCurrent(context.Context) (knowledge.View, error)
}

type Sources struct {
	knowledge Knowledge
}

func NewSources(source Knowledge) (*Sources, error) {
	if source == nil {
		return nil, errors.New("create Entity vector Sources: Knowledge is required")
	}
	return &Sources{knowledge: source}, nil
}

func Namespace(entities []knowledge.Reference[knowledge.EntityID]) (string, error) {
	if err := (knowledge.References{Entities: entities}).Validate(); err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, reference := range entities {
		_, _ = hash.Write([]byte(reference.ID))
		_, _ = hash.Write([]byte{0})
		_, _ = fmt.Fprintf(hash, "%d", reference.Version)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (sources *Sources) Page(
	ctx context.Context,
	entities []knowledge.Reference[knowledge.EntityID],
	offset int,
) (_ Page, resultErr error) {
	if sources == nil || sources.knowledge == nil {
		return Page{}, errors.New("read Entity vector Sources: source is not configured")
	}
	if err := (knowledge.References{Entities: entities}).Validate(); err != nil {
		return Page{}, err
	}
	if offset < 0 || offset > len(entities) {
		return Page{}, errors.New("Entity vector source offset is outside the Target")
	}
	end := min(offset+PageSize, len(entities))
	if offset == end {
		return Page{}, nil
	}
	view, err := sources.knowledge.OpenCurrent(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("open Entity vector Knowledge view: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	versions, err := view.Entities().Read(ctx, entities[offset:end])
	if err != nil {
		return Page{}, fmt.Errorf("read Entity vector source page: %w", err)
	}
	if len(versions) != end-offset {
		return Page{}, fmt.Errorf(
			"read Entity vector source page: expected %d Versions, got %d",
			end-offset,
			len(versions),
		)
	}
	result := Page{
		Items:   make([]Source, len(versions)),
		HasMore: end < len(entities),
	}
	for index, version := range versions {
		reference := entities[offset+index]
		if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
			return Page{}, errors.New("Entity vector source differs from its exact Version reference")
		}
		formatted, err := knowledge.FormatEntityReference(reference)
		if err != nil {
			return Page{}, err
		}
		result.Items[index] = Source{
			ID:   formatted,
			Text: version.Knowledge.Title + ":" + version.Knowledge.Description,
		}
	}
	return result, nil
}
