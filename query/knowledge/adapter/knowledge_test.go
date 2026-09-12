package adapter

import (
	"context"
	"testing"

	domain "github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

const entityID = domain.EntityID("11111111-1111-4111-8111-111111111111")
const otherEntityID = domain.EntityID("22222222-2222-4222-8222-222222222222")
const relationID = domain.RelationID("33333333-3333-4333-8333-333333333333")
const claimID = domain.ClaimID("44444444-4444-4444-8444-444444444444")

func TestReaderMapsEntityPageAtFixedVersions(t *testing.T) {
	version := domain.KnowledgeVersion[domain.Entity]{
		Knowledge: domain.Entity{
			ID: entityID, Title: "Alpha", Type: "person", Aliases: []string{"A"},
			Description: "description",
		},
		Version: 3,
	}
	entities := &activeVersionReader[domain.Entity, domain.EntityID]{
		references: domain.Page[domain.Reference[domain.EntityID], domain.EntityID]{
			Items:     []domain.Reference[domain.EntityID]{{ID: entityID, Version: 3}},
			NextAfter: entityID, HasMore: true,
		},
		versions: []domain.KnowledgeVersion[domain.Entity]{version},
	}
	view := &activeReadView{entities: entities}
	source := &activeViewSource{view: view}
	metadata := &fixedMetadata{
		entity: provenance.EntityMetadata{
			Evidence: []provenance.Evidence{{TextUnitID: "text-1", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
		},
	}
	reader, err := NewReader(source, metadata)
	if err != nil {
		t.Fatal(err)
	}
	versions := domain.Manifest{
		Entities: []domain.Reference[domain.EntityID]{
			{ID: entityID, Version: 3},
		},
	}
	opened, err := reader.Open(t.Context(), versions)
	if err != nil {
		t.Fatal(err)
	}
	page, err := opened.Entities(t.Context(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if source.opens != 1 || view.closes != 1 ||
		len(entities.readReferences) != 1 || entities.readReferences[0].ID != entityID {
		t.Fatalf("fixed read = opens:%d closes:%d refs:%#v",
			source.opens, view.closes, entities.readReferences)
	}
	if len(page.Items) != 1 || page.Items[0].ID != string(entityID) || page.Items[0].Version != 3 ||
		page.Items[0].Degree != 0 || page.Items[0].EvidenceCount != 1 ||
		len(page.Items[0].Aliases) != 1 || page.NextAfter != string(entityID) || page.HasMore {
		t.Fatalf("Entity page = %#v", page)
	}
}

func TestReaderMapsRelationsAndClaimsAtFixedVersions(t *testing.T) {
	relations := &activeVersionReader[domain.Relation, domain.RelationID]{
		references: domain.Page[domain.Reference[domain.RelationID], domain.RelationID]{
			Items: []domain.Reference[domain.RelationID]{{ID: relationID, Version: 2}},
		},
		versions: []domain.KnowledgeVersion[domain.Relation]{{
			Knowledge: domain.Relation{
				ID: relationID, SourceEntityID: entityID, TargetEntityID: otherEntityID,
				Description: "connects",
			},
			Version: 2,
		}},
	}
	claims := &activeVersionReader[domain.Claim, domain.ClaimID]{
		references: domain.Page[domain.Reference[domain.ClaimID], domain.ClaimID]{
			Items: []domain.Reference[domain.ClaimID]{{ID: claimID, Version: 1}},
		},
		versions: []domain.KnowledgeVersion[domain.Claim]{{
			Knowledge: domain.Claim{
				ID: claimID, Subject: relationID, Type: "status", Description: "supported",
			},
			Version: 1,
		}},
	}
	metadata := &fixedMetadata{
		relation: provenance.RelationMetadata{
			Weight:   2,
			Evidence: []provenance.Evidence{{TextUnitID: "text-2", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
		},
		claim: []provenance.ClaimMetadata{{
			SubjectText: "Alpha",
			SourceText:  "source",
			Evidence:    []provenance.Evidence{{TextUnitID: "text-3", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
		}},
	}
	reader, err := NewReader(&activeViewSource{view: &activeReadView{
		relations: relations,
		claims:    claims,
	}}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	view, err := reader.Open(t.Context(), domain.Manifest{
		Relations: []domain.Reference[domain.RelationID]{
			{ID: relationID, Version: 2},
		},
		Claims: []domain.Reference[domain.ClaimID]{
			{ID: claimID, Version: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	relationPage, err := view.Relations(t.Context(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	claimPage, err := view.Claims(t.Context(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if relationPage.Items[0].SourceEntityID != string(entityID) ||
		relationPage.Items[0].TargetEntityID != string(otherEntityID) ||
		relationPage.Items[0].Weight != 2 || relationPage.Items[0].EvidenceCount != 1 ||
		len(relationPage.Items[0].TextUnitIDs) != 1 {
		t.Fatalf("Relation page = %#v", relationPage)
	}
	if claimPage.Items[0].SubjectID != string(relationID) || claimPage.Items[0].Type != "status" ||
		len(claimPage.Items[0].Evidence) != 1 || claimPage.Items[0].Evidence[0].Description != "supported" {
		t.Fatalf("Claim page = %#v", claimPage)
	}
}

type fixedMetadata struct {
	entity   provenance.EntityMetadata
	relation provenance.RelationMetadata
	claim    []provenance.ClaimMetadata
}

func (metadata *fixedMetadata) Entity(
	context.Context,
	domain.Reference[domain.EntityID],
) (provenance.EntityMetadata, error) {
	return metadata.entity, nil
}

func (metadata *fixedMetadata) Relation(
	context.Context,
	domain.Reference[domain.RelationID],
) (provenance.RelationMetadata, error) {
	return metadata.relation, nil
}

func (metadata *fixedMetadata) Claim(
	context.Context,
	domain.Reference[domain.ClaimID],
) ([]provenance.ClaimMetadata, error) {
	return metadata.claim, nil
}

type activeViewSource struct {
	view  domain.View
	opens int
}

func (s *activeViewSource) OpenCurrent(context.Context) (domain.View, error) {
	s.opens++
	return s.view, nil
}

type activeReadView struct {
	entities  domain.VersionReader[domain.Entity, domain.EntityID]
	relations domain.VersionReader[domain.Relation, domain.RelationID]
	claims    domain.VersionReader[domain.Claim, domain.ClaimID]
	closes    int
}

func (v *activeReadView) Identities() domain.IdentityReader { return nil }
func (v *activeReadView) Entities() domain.VersionReader[domain.Entity, domain.EntityID] {
	return v.entities
}
func (v *activeReadView) Relations() domain.VersionReader[domain.Relation, domain.RelationID] {
	return v.relations
}
func (v *activeReadView) Claims() domain.VersionReader[domain.Claim, domain.ClaimID] { return v.claims }
func (v *activeReadView) Close() error {
	v.closes++
	return nil
}

type activeVersionReader[T domain.Knowledge, ID domain.KnowledgeID] struct {
	references     domain.Page[domain.Reference[ID], ID]
	versions       []domain.KnowledgeVersion[T]
	activeLimit    int
	readReferences []domain.Reference[ID]
}

func (r *activeVersionReader[T, ID]) Active(
	_ context.Context,
	_ ID,
	limit int,
) (domain.Page[domain.Reference[ID], ID], error) {
	r.activeLimit = limit
	return r.references, nil
}
func (r *activeVersionReader[T, ID]) Current(context.Context, ID, int) (domain.Page[domain.KnowledgeVersion[T], ID], error) {
	return domain.Page[domain.KnowledgeVersion[T], ID]{}, nil
}
func (r *activeVersionReader[T, ID]) History(context.Context, ID, domain.Version, int) (domain.Page[domain.KnowledgeVersion[T], domain.Version], error) {
	return domain.Page[domain.KnowledgeVersion[T], domain.Version]{}, nil
}
func (r *activeVersionReader[T, ID]) Read(
	_ context.Context,
	references []domain.Reference[ID],
) ([]domain.KnowledgeVersion[T], error) {
	r.readReferences = append([]domain.Reference[ID](nil), references...)
	return append([]domain.KnowledgeVersion[T](nil), r.versions...), nil
}
