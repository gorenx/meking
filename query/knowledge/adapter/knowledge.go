// Package adapter maps exact Knowledge Versions into Query Knowledge browse
// contracts.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"slices"

	domain "github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	queryknowledge "github.com/memoria-space/meking/query/knowledge"
)

type ViewSource interface {
	OpenCurrent(context.Context) (domain.View, error)
}

type Metadata interface {
	Entity(context.Context, domain.Reference[domain.EntityID]) (provenance.EntityMetadata, error)
	Relation(context.Context, domain.Reference[domain.RelationID]) (provenance.RelationMetadata, error)
	Claim(context.Context, domain.Reference[domain.ClaimID]) ([]provenance.ClaimMetadata, error)
}

type Reader struct {
	source   ViewSource
	metadata Metadata
}

func NewReader(source ViewSource, metadata Metadata) (*Reader, error) {
	if source == nil {
		return nil, errors.New("create Query Knowledge reader: source is required")
	}
	if metadata == nil {
		return nil, errors.New("create Query Knowledge reader: Metadata is required")
	}
	return &Reader{
		source:   source,
		metadata: metadata,
	}, nil
}

func (reader *Reader) Open(
	ctx context.Context,
	versions domain.Manifest,
) (queryknowledge.View, error) {
	if reader == nil || reader.source == nil {
		return nil, errors.New("Query Knowledge reader is not configured")
	}
	if err := versions.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", queryknowledge.ErrInvalidRequest, err)
	}
	view, err := reader.source.OpenCurrent(ctx)
	if err != nil {
		return nil, err
	}
	return &viewAdapter{
		view:     view,
		versions: versions.Clone(),
		metadata: reader.metadata,
	}, nil
}

type viewAdapter struct {
	view     domain.View
	versions domain.Manifest
	metadata Metadata
}

func (view *viewAdapter) Entities(
	ctx context.Context,
	after string,
	limit int,
) (queryknowledge.Page[queryknowledge.Entity], error) {
	references, next, more, err := referencePage(view.versions.Entities, domain.EntityID(after), limit)
	if err != nil {
		return queryknowledge.Page[queryknowledge.Entity]{}, err
	}
	versions, err := view.view.Entities().Read(ctx, references)
	if err != nil {
		return queryknowledge.Page[queryknowledge.Entity]{}, err
	}
	items := make([]queryknowledge.Entity, len(versions))
	for index, version := range versions {
		if err := requireVersion(version.Deleted, version.Knowledge.ID, version.Version, references[index]); err != nil {
			return queryknowledge.Page[queryknowledge.Entity]{}, err
		}
		entity := version.Knowledge
		metadata, err := view.metadata.Entity(ctx, references[index])
		if err != nil {
			return queryknowledge.Page[queryknowledge.Entity]{}, err
		}
		items[index] = queryknowledge.Entity{
			ID:            string(entity.ID),
			Version:       uint64(version.Version),
			Title:         entity.Title,
			Type:          entity.Type,
			Aliases:       append([]string(nil), entity.Aliases...),
			Description:   entity.Description,
			EvidenceCount: len(metadata.Evidence),
			TextUnitIDs:   textUnitIDs(metadata.Evidence),
		}
	}
	return queryknowledge.Page[queryknowledge.Entity]{
		Items:     items,
		NextAfter: string(next),
		HasMore:   more,
	}, nil
}

func (view *viewAdapter) Relations(
	ctx context.Context,
	after string,
	limit int,
) (queryknowledge.Page[queryknowledge.Relation], error) {
	references, next, more, err := referencePage(view.versions.Relations, domain.RelationID(after), limit)
	if err != nil {
		return queryknowledge.Page[queryknowledge.Relation]{}, err
	}
	versions, err := view.view.Relations().Read(ctx, references)
	if err != nil {
		return queryknowledge.Page[queryknowledge.Relation]{}, err
	}
	items := make([]queryknowledge.Relation, len(versions))
	for index, version := range versions {
		if err := requireVersion(version.Deleted, version.Knowledge.ID, version.Version, references[index]); err != nil {
			return queryknowledge.Page[queryknowledge.Relation]{}, err
		}
		relation := version.Knowledge
		metadata, err := view.metadata.Relation(ctx, references[index])
		if err != nil {
			return queryknowledge.Page[queryknowledge.Relation]{}, err
		}
		items[index] = queryknowledge.Relation{
			ID:             string(relation.ID),
			Version:        uint64(version.Version),
			SourceEntityID: string(relation.SourceEntityID),
			TargetEntityID: string(relation.TargetEntityID),
			Description:    relation.Description,
			Weight:         metadata.Weight,
			EvidenceCount:  len(metadata.Evidence),
			TextUnitIDs:    textUnitIDs(metadata.Evidence),
		}
	}
	return queryknowledge.Page[queryknowledge.Relation]{
		Items:     items,
		NextAfter: string(next),
		HasMore:   more,
	}, nil
}

func (view *viewAdapter) Claims(
	ctx context.Context,
	after string,
	limit int,
) (queryknowledge.Page[queryknowledge.Claim], error) {
	references, next, more, err := referencePage(view.versions.Claims, domain.ClaimID(after), limit)
	if err != nil {
		return queryknowledge.Page[queryknowledge.Claim]{}, err
	}
	versions, err := view.view.Claims().Read(ctx, references)
	if err != nil {
		return queryknowledge.Page[queryknowledge.Claim]{}, err
	}
	items := make([]queryknowledge.Claim, len(versions))
	for index, version := range versions {
		if err := requireVersion(version.Deleted, version.Knowledge.ID, version.Version, references[index]); err != nil {
			return queryknowledge.Page[queryknowledge.Claim]{}, err
		}
		claim := version.Knowledge
		metadata, err := view.metadata.Claim(ctx, references[index])
		if err != nil {
			return queryknowledge.Page[queryknowledge.Claim]{}, err
		}
		evidenceItems := make([]queryknowledge.ClaimEvidence, 0)
		for _, source := range metadata {
			for _, evidence := range source.Evidence {
				evidenceItems = append(evidenceItems, queryknowledge.ClaimEvidence{
					TextUnitID:  evidence.TextUnitID,
					SubjectText: source.SubjectText,
					ObjectText:  source.ObjectText,
					Status:      source.Status,
					StartDate:   source.StartDate,
					EndDate:     source.EndDate,
					Description: claim.Description,
					SourceText:  source.SourceText,
				})
			}
		}
		items[index] = queryknowledge.Claim{
			ID:        string(claim.ID),
			Version:   uint64(version.Version),
			SubjectID: claim.Subject.ObjectID(),
			Type:      claim.Type,
			Evidence:  evidenceItems,
		}
	}
	return queryknowledge.Page[queryknowledge.Claim]{
		Items:     items,
		NextAfter: string(next),
		HasMore:   more,
	}, nil
}

func textUnitIDs(evidence []provenance.Evidence) []string {
	unique := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		unique[item.TextUnitID] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for textUnitID := range unique {
		result = append(result, textUnitID)
	}
	slices.Sort(result)
	return result
}

func referencePage[ID domain.KnowledgeID](
	references []domain.Reference[ID],
	after ID,
	limit int,
) ([]domain.Reference[ID], ID, bool, error) {
	if limit < 1 || limit > queryknowledge.MaximumPageSize {
		return nil, after, false, queryknowledge.ErrInvalidRequest
	}
	start := 0
	for start < len(references) && references[start].ID <= after {
		start++
	}
	end := min(start+limit, len(references))
	items := append([]domain.Reference[ID](nil), references[start:end]...)
	next := after
	if len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return items, next, end < len(references), nil
}

func requireVersion[ID domain.KnowledgeID](
	deleted bool,
	id ID,
	version domain.Version,
	reference domain.Reference[ID],
) error {
	if deleted || id != reference.ID || version != reference.Version {
		return errors.New("Query Knowledge differs from its exact Version reference")
	}
	return nil
}

func (view *viewAdapter) Close() error {
	if view == nil || view.view == nil {
		return nil
	}
	err := view.view.Close()
	view.view = nil
	return err
}

var _ queryknowledge.Reader = (*Reader)(nil)
var _ queryknowledge.View = (*viewAdapter)(nil)
