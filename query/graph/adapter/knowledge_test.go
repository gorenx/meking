package adapter

import (
	"context"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
)

const (
	graphEntityID   = knowledge.EntityID("11111111-1111-4111-8111-111111111111")
	graphOtherID    = knowledge.EntityID("22222222-2222-4222-8222-222222222222")
	graphRelationID = knowledge.RelationID("33333333-3333-4333-8333-333333333333")
)

func TestKnowledgeReaderFixesSourceAndMapsCurrentPages(t *testing.T) {
	entities := &graphVersionReader[knowledge.Entity, knowledge.EntityID]{
		values: []knowledge.KnowledgeVersion[knowledge.Entity]{{
			Knowledge: knowledge.Entity{
				ID: graphEntityID, Title: "Alpha", Type: "person", Aliases: []string{"A"},
				Description: "entity",
			},
			Version: 3,
		}}}
	relations := &graphVersionReader[knowledge.Relation, knowledge.RelationID]{values: []knowledge.KnowledgeVersion[knowledge.Relation]{{
		Knowledge: knowledge.Relation{
			ID: graphRelationID, SourceEntityID: graphEntityID, TargetEntityID: graphOtherID,
			Type: "works_with", Description: "relation",
		},
		Version: 2,
	}}}
	view := &graphReadView{entities: entities, relations: relations}
	provider := &graphKnowledgeSource{view: view}
	metadata := &graphMetadata{}
	reader, err := NewKnowledgeReader(provider, metadata)
	if err != nil {
		t.Fatalf("NewKnowledgeReader() error = %v", err)
	}
	opened, err := reader.OpenCurrent(t.Context())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	entityPage, err := opened.Entities(t.Context(), "", 20)
	if err != nil {
		t.Fatalf("Entities() error = %v", err)
	}
	relationPage, err := opened.Relations(t.Context(), "", 20)
	if err != nil {
		t.Fatalf("Relations() error = %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if provider.opens != 1 || view.closes != 1 {
		t.Fatalf("fixed source/close = %d/%d", provider.opens, view.closes)
	}
	if len(entityPage.Items) != 1 || entityPage.Items[0].ID != string(graphEntityID) ||
		entityPage.Items[0].Version != 3 || entityPage.Items[0].Degree != 1 ||
		entityPage.Items[0].TextUnitCount != 1 ||
		!reflect.DeepEqual(entityPage.Items[0].Aliases, []string{"A"}) ||
		entityPage.NextAfter != "" || entityPage.HasMore {
		t.Fatalf("Entity page = %#v", entityPage)
	}
	if len(relationPage.Items) != 1 || relationPage.Items[0].ID != string(graphRelationID) ||
		relationPage.Items[0].Type != "works_with" ||
		relationPage.Items[0].CombinedDegree != 2 || relationPage.Items[0].Weight != 2 ||
		relationPage.Items[0].TextUnitCount != 1 ||
		relationPage.NextAfter != "" || relationPage.HasMore {
		t.Fatalf("Relation page = %#v", relationPage)
	}
}

type graphMetadata struct{}

func (*graphMetadata) Entity(
	context.Context,
	knowledge.Reference[knowledge.EntityID],
) (provenance.EntityMetadata, error) {
	return provenance.EntityMetadata{
		Evidence: []provenance.Evidence{{TextUnitID: "text-1", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
	}, nil
}

func (*graphMetadata) Relation(
	context.Context,
	knowledge.Reference[knowledge.RelationID],
) (provenance.RelationMetadata, error) {
	return provenance.RelationMetadata{
		Weight:   2,
		Evidence: []provenance.Evidence{{TextUnitID: "text-2", Source: provenance.CorporaSource{CorporaID: "corpora"}}},
	}, nil
}

type graphKnowledgeSource struct {
	view  knowledge.View
	opens int
}

func (c *graphKnowledgeSource) OpenCurrent(context.Context) (knowledge.View, error) {
	c.opens++
	return c.view, nil
}

type graphReadView struct {
	entities  knowledge.VersionReader[knowledge.Entity, knowledge.EntityID]
	relations knowledge.VersionReader[knowledge.Relation, knowledge.RelationID]
	closes    int
}

func (v *graphReadView) Identities() knowledge.IdentityReader { return nil }
func (v *graphReadView) Entities() knowledge.VersionReader[knowledge.Entity, knowledge.EntityID] {
	return v.entities
}
func (v *graphReadView) Relations() knowledge.VersionReader[knowledge.Relation, knowledge.RelationID] {
	return v.relations
}
func (v *graphReadView) Claims() knowledge.VersionReader[knowledge.Claim, knowledge.ClaimID] {
	return nil
}
func (v *graphReadView) Close() error {
	v.closes++
	return nil
}

type graphVersionReader[T knowledge.Knowledge, ID knowledge.KnowledgeID] struct {
	values []knowledge.KnowledgeVersion[T]
}

func (r *graphVersionReader[T, ID]) Active(
	context.Context,
	ID,
	int,
) (knowledge.Page[knowledge.Reference[ID], ID], error) {
	return knowledge.Page[knowledge.Reference[ID], ID]{}, nil
}

func (r *graphVersionReader[T, ID]) Current(
	context.Context,
	ID,
	int,
) (knowledge.Page[knowledge.KnowledgeVersion[T], ID], error) {
	return knowledge.Page[knowledge.KnowledgeVersion[T], ID]{
		Items: append([]knowledge.KnowledgeVersion[T](nil), r.values...),
	}, nil
}
func (r *graphVersionReader[T, ID]) History(
	context.Context,
	ID,
	knowledge.Version,
	int,
) (knowledge.Page[knowledge.KnowledgeVersion[T], knowledge.Version], error) {
	return knowledge.Page[knowledge.KnowledgeVersion[T], knowledge.Version]{}, nil
}
func (r *graphVersionReader[T, ID]) Read(
	context.Context,
	[]knowledge.Reference[ID],
) ([]knowledge.KnowledgeVersion[T], error) {
	return append([]knowledge.KnowledgeVersion[T](nil), r.values...), nil
}
