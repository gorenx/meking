// Package knowledge maps exact Knowledge Versions into Community-owned read
// contracts.
package knowledge

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community/structurebuild"
	knowledgebase "github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

type Metadata interface {
	Entity(
		context.Context,
		knowledgebase.Reference[knowledgebase.EntityID],
	) (provenance.EntityMetadata, error)
	Relation(
		context.Context,
		knowledgebase.Reference[knowledgebase.RelationID],
	) (provenance.RelationMetadata, error)
	Claim(
		context.Context,
		knowledgebase.Reference[knowledgebase.ClaimID],
	) ([]provenance.ClaimMetadata, error)
}

type Reader struct {
	source   *knowledgebase.Reader
	metadata Metadata
}

func NewReader(source *knowledgebase.Reader, metadata Metadata) (*Reader, error) {
	if source == nil {
		return nil, errors.New("create Community Knowledge Reader: source is required")
	}
	if metadata == nil {
		return nil, errors.New("create Community Knowledge Reader: Metadata is required")
	}
	return &Reader{source: source, metadata: metadata}, nil
}

func (reader *Reader) Snapshot(
	ctx context.Context,
	versions knowledgebase.Manifest,
) (_ structurebuild.KnowledgeSnapshot, resultErr error) {
	if reader == nil || reader.source == nil {
		return structurebuild.KnowledgeSnapshot{}, errors.New(
			"read Community Knowledge snapshot: Reader is not initialized",
		)
	}
	if err := versions.Validate(); err != nil {
		return structurebuild.KnowledgeSnapshot{}, err
	}
	view, err := reader.source.OpenCurrent(ctx)
	if err != nil {
		return structurebuild.KnowledgeSnapshot{}, fmt.Errorf("open Community Knowledge: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, view.Close())
	}()
	entities, err := view.Entities().Read(ctx, versions.Entities)
	if err != nil {
		return structurebuild.KnowledgeSnapshot{}, fmt.Errorf("read Community Entity Versions: %w", err)
	}
	relations, err := view.Relations().Read(ctx, versions.Relations)
	if err != nil {
		return structurebuild.KnowledgeSnapshot{}, fmt.Errorf("read Community Relation Versions: %w", err)
	}
	if len(entities) != len(versions.Entities) || len(relations) != len(versions.Relations) {
		return structurebuild.KnowledgeSnapshot{}, errors.New("Community Knowledge snapshot is incomplete")
	}
	result := structurebuild.KnowledgeSnapshot{
		Entities:  make([]structurebuild.Entity, len(entities)),
		Relations: make([]structurebuild.Relation, len(relations)),
	}
	for index, version := range entities {
		reference := versions.Entities[index]
		if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
			return structurebuild.KnowledgeSnapshot{}, errors.New("Community Entity differs from its exact Version reference")
		}
		result.Entities[index] = structurebuild.Entity{
			ID:      string(reference.ID),
			Version: uint64(reference.Version),
		}
	}
	for index, version := range relations {
		reference := versions.Relations[index]
		if version.Deleted || version.Knowledge.ID != reference.ID || version.Version != reference.Version {
			return structurebuild.KnowledgeSnapshot{}, errors.New("Community Relation differs from its exact Version reference")
		}
		metadata, err := reader.metadata.Relation(ctx, reference)
		if err != nil {
			return structurebuild.KnowledgeSnapshot{}, err
		}
		result.Relations[index] = structurebuild.Relation{
			ID:             string(reference.ID),
			Version:        uint64(reference.Version),
			SourceEntityID: string(version.Knowledge.SourceEntityID),
			TargetEntityID: string(version.Knowledge.TargetEntityID),
			Weight:         metadata.Weight,
		}
	}
	return result, nil
}
