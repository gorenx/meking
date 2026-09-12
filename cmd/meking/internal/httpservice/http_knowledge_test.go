package httpservice

import (
	"context"
	"net/http"
	"testing"

	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	querysource "github.com/memoria-space/meking/query/source"
)

func TestHTTPKnowledgeEntityPageMapsFixedPublicationAndCursor(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	browser := &testKnowledgeBrowser{}
	browser.entities = func(
		_ context.Context,
		request queryknowledge.PageRequest,
	) (queryknowledge.EntityPage, error) {
		if request.After != "entity-before" || request.Limit != 12 {
			t.Fatalf("Entity page request = %#v", request)
		}
		return queryknowledge.EntityPage{
			Publication: queryknowledge.Publication{
				EpochID: 9, CorporaID: "corpus-1", ReportSetID: "reports-1",
			},
			Items: []queryknowledge.Entity{{
				ID: "entity-1", Version: 3, Title: "Alpha", Type: "person",
				Aliases: []string{}, Degree: 8, EvidenceCount: 2,
			}},
			NextAfter: "entity-1", HasMore: true,
		}, nil
	}
	dependencies.knowledge = browser
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/knowledge/entities?after=entity-before&limit=12", nil,
	)
	var body httpKnowledgeEntityPage
	decodeHTTPTestResponse(t, response, &body)
	if response.Code != http.StatusOK || body.EpochID != 9 ||
		body.CorporaID != "corpus-1" || len(body.Items) != 1 || body.Items[0].Version != 3 ||
		body.Items[0].Aliases == nil || body.Items[0].EvidenceCount != 2 ||
		body.NextAfter != "entity-1" || !body.HasMore {
		t.Fatalf("Entity page response = %d/%#v", response.Code, body)
	}
}

func TestHTTPKnowledgeRelationPageMapsFixedPublicationAndCursor(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	browser := &testKnowledgeBrowser{}
	browser.relations = func(
		_ context.Context,
		request queryknowledge.PageRequest,
	) (queryknowledge.RelationPage, error) {
		if request.After != "relation-before" || request.Limit != 12 {
			t.Fatalf("Relation page request = %#v", request)
		}
		return queryknowledge.RelationPage{
			Publication: queryknowledge.Publication{
				EpochID: 9, CorporaID: "corpus-1", ReportSetID: "reports-1",
			},
			Items: []queryknowledge.Relation{{
				ID: "relation-1", Version: 3, SourceEntityID: "entity-1", TargetEntityID: "entity-2",
				Weight: 2.5, CombinedDegree: 10, EvidenceCount: 4,
			}},
			NextAfter: "relation-1", HasMore: true,
		}, nil
	}
	dependencies.knowledge = browser
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/knowledge/relations?after=relation-before&limit=12", nil,
	)
	var body httpKnowledgeRelationPage
	decodeHTTPTestResponse(t, response, &body)
	if response.Code != http.StatusOK || body.EpochID != 9 || len(body.Items) != 1 ||
		body.Items[0].SourceEntityID != "entity-1" || body.Items[0].EvidenceCount != 4 ||
		body.NextAfter != "relation-1" || !body.HasMore {
		t.Fatalf("Relation page response = %d/%#v", response.Code, body)
	}
}

func TestHTTPKnowledgeClaimPageMapsFixedPublicationAndStatementSources(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	browser := &testKnowledgeBrowser{}
	browser.claims = func(
		_ context.Context,
		request queryknowledge.PageRequest,
	) (queryknowledge.ClaimPage, error) {
		if request.After != "claim-before" || request.Limit != 12 {
			t.Fatalf("Claim page request = %#v", request)
		}
		return queryknowledge.ClaimPage{
			Publication: queryknowledge.Publication{
				EpochID: 9, CorporaID: "corpus-1", ReportSetID: "reports-1",
			},
			Items: []queryknowledge.Claim{{
				ID: "claim-1", Version: 2, SubjectID: "entity-1", Type: "status",
				Evidence: []queryknowledge.ClaimEvidence{{
					TextUnitID: "unit-1", SubjectText: "Alpha", ObjectText: "active",
					Status: "true", Description: "supported", SourceText: "source",
				}},
			}},
			NextAfter: "claim-1", HasMore: true,
		}, nil
	}
	dependencies.knowledge = browser
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/knowledge/claims?after=claim-before&limit=12", nil,
	)
	var body httpKnowledgeClaimPage
	decodeHTTPTestResponse(t, response, &body)
	if response.Code != http.StatusOK || body.EpochID != 9 || len(body.Items) != 1 ||
		body.Items[0].SubjectID != "entity-1" || len(body.Items[0].Evidence) != 1 ||
		body.Items[0].Evidence[0].TextUnitID != "unit-1" ||
		body.NextAfter != "claim-1" || !body.HasMore {
		t.Fatalf("Claim page response = %d/%#v", response.Code, body)
	}
}

func TestHTTPKnowledgeDetailsMapAssociatedObjectsAndTextUnits(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	browser := &testKnowledgeBrowser{}
	browser.entity = func(_ context.Context, id string, epochID int64) (queryknowledge.EntityDetail, error) {
		if id != "entity-1" || epochID != 9 {
			t.Fatalf("Entity detail identity = %q/Epoch %d", id, epochID)
		}
		return queryknowledge.EntityDetail{
			Publication: queryknowledge.Publication{EpochID: 9, CorporaID: "corpus-1"},
			Entity:      queryknowledge.Entity{ID: id, Version: 2, Title: "Alpha", Aliases: []string{}},
			Relations: []queryknowledge.Relation{{
				ID: "relation-1", Version: 3, SourceEntityID: id, TargetEntityID: "entity-2",
			}},
			Neighbors: []queryknowledge.Entity{{ID: "entity-2", Version: 1, Title: "Beta", Aliases: []string{}}},
			Claims: []queryknowledge.Claim{{
				ID: "claim-1", Version: 1, SubjectID: id, Type: "status",
				Evidence: []queryknowledge.ClaimEvidence{{TextUnitID: "unit-1", SourceText: "source"}},
			}},
			TextUnits: []querysource.TextUnitSource{{
				CorporaID: "corpus-1", TextUnitID: "unit-1", Text: "evidence", TextID: "text-1",
				DocumentID: "document-1", DocumentLocation: "source.md", TextTitle: "Source",
			}},
		}, nil
	}
	browser.relation = func(_ context.Context, id string, epochID int64) (queryknowledge.RelationDetail, error) {
		if epochID != 9 {
			t.Fatalf("Relation detail Epoch = %d", epochID)
		}
		return queryknowledge.RelationDetail{
			Publication: queryknowledge.Publication{EpochID: 9, CorporaID: "corpus-1"},
			Relation:    queryknowledge.Relation{ID: id, SourceEntityID: "entity-1", TargetEntityID: "entity-2"},
			Endpoints:   []queryknowledge.Entity{{ID: "entity-1", Aliases: []string{}}, {ID: "entity-2", Aliases: []string{}}},
			Claims:      []queryknowledge.Claim{}, TextUnits: []querysource.TextUnitSource{},
		}, nil
	}
	dependencies.knowledge = browser
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	entityResponse := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/knowledge/entities/entity-1?epoch_id=9", nil)
	var entity httpKnowledgeEntityDetail
	decodeHTTPTestResponse(t, entityResponse, &entity)
	if entityResponse.Code != http.StatusOK || entity.Entity.ID != "entity-1" ||
		len(entity.Relations) != 1 || len(entity.Neighbors) != 1 || len(entity.Claims) != 1 ||
		len(entity.TextUnits) != 1 || entity.TextUnits[0].Text != "evidence" {
		t.Fatalf("Entity detail response = %d/%#v", entityResponse.Code, entity)
	}
	relationResponse := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/knowledge/relations/relation-1?epoch_id=9", nil)
	var relation httpKnowledgeRelationDetail
	decodeHTTPTestResponse(t, relationResponse, &relation)
	if relationResponse.Code != http.StatusOK || relation.Relation.ID != "relation-1" ||
		len(relation.Endpoints) != 2 || relation.Claims == nil || relation.TextUnits == nil {
		t.Fatalf("Relation detail response = %d/%#v", relationResponse.Code, relation)
	}
}

func TestHTTPKnowledgeEntityPageRejectsInvalidLimit(t *testing.T) {
	handler := mustHTTPHandler(t, httpTestConfig(t), defaultHTTPDependencies())
	response := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/knowledge/entities?limit=201", nil,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit response = %d/%s", response.Code, response.Body.String())
	}
}

func TestHTTPKnowledgeDetailRequiresEpochID(t *testing.T) {
	handler := mustHTTPHandler(t, httpTestConfig(t), defaultHTTPDependencies())
	response := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/knowledge/entities/entity-1", nil,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing Epoch response = %d/%s", response.Code, response.Body.String())
	}
}

type testKnowledgeBrowser struct {
	entities  func(context.Context, queryknowledge.PageRequest) (queryknowledge.EntityPage, error)
	relations func(context.Context, queryknowledge.PageRequest) (queryknowledge.RelationPage, error)
	claims    func(context.Context, queryknowledge.PageRequest) (queryknowledge.ClaimPage, error)
	entity    func(context.Context, string, int64) (queryknowledge.EntityDetail, error)
	relation  func(context.Context, string, int64) (queryknowledge.RelationDetail, error)
}

func (b *testKnowledgeBrowser) BrowseRelations(
	ctx context.Context,
	request queryknowledge.PageRequest,
) (queryknowledge.RelationPage, error) {
	return b.relations(ctx, request)
}

func (b *testKnowledgeBrowser) BrowseClaims(
	ctx context.Context,
	request queryknowledge.PageRequest,
) (queryknowledge.ClaimPage, error) {
	return b.claims(ctx, request)
}

func (b *testKnowledgeBrowser) Entity(ctx context.Context, id string, epochID int64) (queryknowledge.EntityDetail, error) {
	return b.entity(ctx, id, epochID)
}

func (b *testKnowledgeBrowser) Relation(ctx context.Context, id string, epochID int64) (queryknowledge.RelationDetail, error) {
	return b.relation(ctx, id, epochID)
}

func (b *testKnowledgeBrowser) BrowseEntities(
	ctx context.Context,
	request queryknowledge.PageRequest,
) (queryknowledge.EntityPage, error) {
	return b.entities(ctx, request)
}
