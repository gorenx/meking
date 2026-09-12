package adapter

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	querybase "github.com/memoria-space/meking/query"
	querylocal "github.com/memoria-space/meking/query/local"
)

const (
	localEntityID      = knowledge.EntityID("11111111-1111-4111-8111-111111111111")
	localOtherEntityID = knowledge.EntityID("22222222-2222-4222-8222-222222222222")
	localRelationID    = knowledge.RelationID("33333333-3333-4333-8333-333333333333")
	localClaimID       = knowledge.ClaimID("44444444-4444-4444-8444-444444444444")
)

type localKnowledgeSource struct {
	view  knowledge.View
	opens int
}

func (c *localKnowledgeSource) OpenCurrent(context.Context) (knowledge.View, error) {
	c.opens++
	return c.view, nil
}

type localReadView struct {
	entities  knowledge.VersionReader[knowledge.Entity, knowledge.EntityID]
	relations knowledge.VersionReader[knowledge.Relation, knowledge.RelationID]
	claims    knowledge.VersionReader[knowledge.Claim, knowledge.ClaimID]
	closes    int
}

func (v *localReadView) Identities() knowledge.IdentityReader { return nil }
func (v *localReadView) Entities() knowledge.VersionReader[knowledge.Entity, knowledge.EntityID] {
	return v.entities
}
func (v *localReadView) Relations() knowledge.VersionReader[knowledge.Relation, knowledge.RelationID] {
	return v.relations
}
func (v *localReadView) Claims() knowledge.VersionReader[knowledge.Claim, knowledge.ClaimID] {
	return v.claims
}
func (v *localReadView) Close() error { v.closes++; return nil }

type localVersionReader[T knowledge.Knowledge, ID knowledge.KnowledgeID] struct {
	requested []knowledge.Reference[ID]
	values    []knowledge.KnowledgeVersion[T]
}

func (r *localVersionReader[T, ID]) Active(context.Context, ID, int) (knowledge.Page[knowledge.Reference[ID], ID], error) {
	return knowledge.Page[knowledge.Reference[ID], ID]{}, errors.New("unexpected Active read")
}

func (r *localVersionReader[T, ID]) Current(context.Context, ID, int) (knowledge.Page[knowledge.KnowledgeVersion[T], ID], error) {
	return knowledge.Page[knowledge.KnowledgeVersion[T], ID]{}, errors.New("unexpected Current read")
}
func (r *localVersionReader[T, ID]) History(context.Context, ID, knowledge.Version, int) (knowledge.Page[knowledge.KnowledgeVersion[T], knowledge.Version], error) {
	return knowledge.Page[knowledge.KnowledgeVersion[T], knowledge.Version]{}, errors.New("unexpected History read")
}
func (r *localVersionReader[T, ID]) Read(
	_ context.Context,
	references []knowledge.Reference[ID],
) ([]knowledge.KnowledgeVersion[T], error) {
	r.requested = append([]knowledge.Reference[ID](nil), references...)
	return append([]knowledge.KnowledgeVersion[T](nil), r.values...), nil
}

func TestKnowledgeReaderUsesOneViewAndExactVersions(t *testing.T) {
	subject, err := knowledge.NewEntitySubject(localEntityID)
	if err != nil {
		t.Fatalf("NewEntitySubject() error = %v", err)
	}
	entities := &localVersionReader[knowledge.Entity, knowledge.EntityID]{values: []knowledge.KnowledgeVersion[knowledge.Entity]{{
		Knowledge: knowledge.Entity{
			ID: localEntityID, Title: "Alpha", Description: "entity",
		},
		Version: 4,
	}}}
	relations := &localVersionReader[knowledge.Relation, knowledge.RelationID]{values: []knowledge.KnowledgeVersion[knowledge.Relation]{{
		Knowledge: knowledge.Relation{
			ID: localRelationID, SourceEntityID: localEntityID, TargetEntityID: localOtherEntityID,
			Description: "relation",
		},
		Version: 2,
	}}}
	claims := &localVersionReader[knowledge.Claim, knowledge.ClaimID]{values: []knowledge.KnowledgeVersion[knowledge.Claim]{{
		Knowledge: knowledge.Claim{
			ID: localClaimID, Subject: subject, Type: "status", Description: "first",
		},
		Version: 3,
	}}}
	view := &localReadView{entities: entities, relations: relations, claims: claims}
	catalog := &localKnowledgeSource{view: view}
	reader, err := NewKnowledgeReader(catalog, localMetadata{})
	if err != nil {
		t.Fatalf("NewKnowledgeReader() error = %v", err)
	}
	request := querylocal.KnowledgeRequest{
		Entities:  []querybase.KnowledgeReference{{ID: string(localEntityID), Version: 4}},
		Relations: []querybase.KnowledgeReference{{ID: string(localRelationID), Version: 2}},
		Claims: []querybase.ClaimReference{
			{ID: string(localClaimID), Version: 3, EvidenceIndex: 0},
		},
	}
	versions := knowledge.Manifest{
		Entities: []knowledge.Reference[knowledge.EntityID]{
			{ID: localEntityID, Version: 4},
		},
		Relations: []knowledge.Reference[knowledge.RelationID]{
			{ID: localRelationID, Version: 2},
		},
		Claims: []knowledge.Reference[knowledge.ClaimID]{
			{ID: localClaimID, Version: 3},
		},
	}
	result, err := reader.Read(t.Context(), versions, request)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if catalog.opens != 1 || view.closes != 1 {
		t.Fatalf("view lifecycle = opens:%d closes:%d", catalog.opens, view.closes)
	}
	if !reflect.DeepEqual(entities.requested, []knowledge.Reference[knowledge.EntityID]{{ID: localEntityID, Version: 4}}) ||
		!reflect.DeepEqual(relations.requested, []knowledge.Reference[knowledge.RelationID]{{ID: localRelationID, Version: 2}}) ||
		!reflect.DeepEqual(claims.requested, []knowledge.Reference[knowledge.ClaimID]{{ID: localClaimID, Version: 3}}) {
		t.Fatalf("exact requests = entities:%#v relations:%#v claims:%#v",
			entities.requested, relations.requested, claims.requested)
	}
	if len(result.Entities) != 1 || result.Entities[0].Version != 4 ||
		len(result.Relationships) != 1 || result.Relationships[0].Version != 2 ||
		len(result.Claims) != 1 || result.Claims[0].EvidenceIndex != 0 ||
		result.Claims[0].Description != "first" {
		t.Fatalf("mapped result = %#v", result)
	}
}

type localMetadata struct{}

func (localMetadata) Entity(
	context.Context,
	knowledge.Reference[knowledge.EntityID],
) (provenance.EntityMetadata, error) {
	return provenance.EntityMetadata{
		Evidence: []provenance.Evidence{{TextUnitID: "text-1", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
	}, nil
}

func (localMetadata) Relation(
	context.Context,
	knowledge.Reference[knowledge.RelationID],
) (provenance.RelationMetadata, error) {
	return provenance.RelationMetadata{
		Weight:   2,
		Evidence: []provenance.Evidence{{TextUnitID: "text-2", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
	}, nil
}

func (localMetadata) Claim(
	context.Context,
	knowledge.Reference[knowledge.ClaimID],
) ([]provenance.ClaimMetadata, error) {
	return []provenance.ClaimMetadata{{
		SourceText: "source",
		Evidence:   []provenance.Evidence{{TextUnitID: "text-3", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
	}}, nil
}
