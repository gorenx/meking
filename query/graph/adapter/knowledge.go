// Package adapter maps provider domain applications into graph-owned read ports.
package adapter

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	querygraph "github.com/memoria-space/meking/query/graph"
)

type KnowledgeViewSource interface {
	OpenCurrent(context.Context) (knowledge.View, error)
}

type Metadata interface {
	Entity(context.Context, knowledge.Reference[knowledge.EntityID]) (provenance.EntityMetadata, error)
	Relation(context.Context, knowledge.Reference[knowledge.RelationID]) (provenance.RelationMetadata, error)
}

type KnowledgeReader struct {
	source   KnowledgeViewSource
	metadata Metadata
}

func NewKnowledgeReader(source KnowledgeViewSource, metadata Metadata) (*KnowledgeReader, error) {
	if source == nil {
		return nil, errors.New("create graph Knowledge reader: Knowledge view source is required")
	}
	if metadata == nil {
		return nil, errors.New("create graph Knowledge reader: Metadata is required")
	}
	return &KnowledgeReader{
		source:   source,
		metadata: metadata,
	}, nil
}

func (reader *KnowledgeReader) OpenCurrent(ctx context.Context) (querygraph.KnowledgeView, error) {
	if reader == nil || reader.source == nil {
		return nil, errors.New("graph Knowledge reader is not configured")
	}
	view, err := reader.source.OpenCurrent(ctx)
	if err != nil {
		return nil, err
	}
	degrees, err := currentDegrees(ctx, view.Relations())
	if err != nil {
		_ = view.Close()
		return nil, err
	}
	return &knowledgeView{
		view:     view,
		metadata: reader.metadata,
		degrees:  degrees,
	}, nil
}

type knowledgeView struct {
	view     knowledge.View
	metadata Metadata
	degrees  map[knowledge.EntityID]int
}

func (view *knowledgeView) Entities(
	ctx context.Context,
	after string,
	limit int,
) (querygraph.KnowledgePage[querygraph.KnowledgeEntity], error) {
	page, err := view.view.Entities().Current(ctx, knowledge.EntityID(after), limit)
	if err != nil {
		return querygraph.KnowledgePage[querygraph.KnowledgeEntity]{}, err
	}
	result := make([]querygraph.KnowledgeEntity, len(page.Items))
	for index, version := range page.Items {
		entity := version.Knowledge
		textUnitCount := 0
		if !version.Deleted {
			metadata, err := view.metadata.Entity(ctx, knowledge.Reference[knowledge.EntityID]{
				ID: entity.ID, Version: version.Version,
			})
			if err != nil {
				return querygraph.KnowledgePage[querygraph.KnowledgeEntity]{}, err
			}
			textUnitCount = len(metadata.Evidence)
		}
		result[index] = querygraph.KnowledgeEntity{
			ID:            string(entity.ID),
			Version:       uint64(version.Version),
			Deleted:       version.Deleted,
			Title:         entity.Title,
			Type:          entity.Type,
			Aliases:       append([]string(nil), entity.Aliases...),
			Description:   entity.Description,
			Degree:        view.degrees[entity.ID],
			TextUnitCount: textUnitCount,
		}
	}
	return querygraph.KnowledgePage[querygraph.KnowledgeEntity]{
		Items:     result,
		NextAfter: string(page.NextAfter),
		HasMore:   page.HasMore,
	}, nil
}

func (view *knowledgeView) Relations(
	ctx context.Context,
	after string,
	limit int,
) (querygraph.KnowledgePage[querygraph.KnowledgeRelation], error) {
	page, err := view.view.Relations().Current(ctx, knowledge.RelationID(after), limit)
	if err != nil {
		return querygraph.KnowledgePage[querygraph.KnowledgeRelation]{}, err
	}
	result := make([]querygraph.KnowledgeRelation, len(page.Items))
	for index, version := range page.Items {
		relation := version.Knowledge
		weight := float64(0)
		textUnitCount := 0
		if !version.Deleted {
			metadata, err := view.metadata.Relation(ctx, knowledge.Reference[knowledge.RelationID]{
				ID: relation.ID, Version: version.Version,
			})
			if err != nil {
				return querygraph.KnowledgePage[querygraph.KnowledgeRelation]{}, err
			}
			weight = metadata.Weight
			textUnitCount = len(metadata.Evidence)
		}
		result[index] = querygraph.KnowledgeRelation{
			ID:             string(relation.ID),
			Version:        uint64(version.Version),
			Deleted:        version.Deleted,
			SourceEntityID: string(relation.SourceEntityID),
			TargetEntityID: string(relation.TargetEntityID),
			Type:           relation.Type,
			Description:    relation.Description,
			Weight:         weight,
			CombinedDegree: view.degrees[relation.SourceEntityID] + view.degrees[relation.TargetEntityID],
			TextUnitCount:  textUnitCount,
		}
	}
	return querygraph.KnowledgePage[querygraph.KnowledgeRelation]{
		Items:     result,
		NextAfter: string(page.NextAfter),
		HasMore:   page.HasMore,
	}, nil
}

func currentDegrees(
	ctx context.Context,
	relations knowledge.VersionReader[knowledge.Relation, knowledge.RelationID],
) (map[knowledge.EntityID]int, error) {
	result := make(map[knowledge.EntityID]int)
	for after := knowledge.RelationID(""); ; {
		page, err := relations.Current(ctx, after, 512)
		if err != nil {
			return nil, err
		}
		for _, version := range page.Items {
			if version.Deleted {
				continue
			}
			result[version.Knowledge.SourceEntityID]++
			result[version.Knowledge.TargetEntityID]++
		}
		if !page.HasMore {
			return result, nil
		}
		if page.NextAfter == "" || page.NextAfter == after {
			return nil, errors.New("read current Knowledge Relations: provider returned an invalid next cursor")
		}
		after = page.NextAfter
	}
}

func (view *knowledgeView) Close() error {
	if view == nil || view.view == nil {
		return nil
	}
	err := view.view.Close()
	view.view = nil
	return err
}

var _ querygraph.KnowledgeReader = (*KnowledgeReader)(nil)
var _ querygraph.KnowledgeView = (*knowledgeView)(nil)
