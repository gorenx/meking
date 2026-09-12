package httpservice

import (
	"context"
	"net/http"
	"testing"

	querygraph "github.com/memoria-space/meking/query/graph"
)

func TestHTTPGraphEndpointsMapKnowledgeAndCommunityGraphs(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	graphs := &testGraphBrowser{}
	graphs.browseEntities = func(
		_ context.Context,
		request querygraph.EntityGraphRequest,
	) (querygraph.EntityGraph, error) {
		if request.Query != "alpha" || request.EntityType != "person" || request.Limit != 12 {
			t.Fatalf("Entity request = %#v", request)
		}
		return querygraph.EntityGraph{
			Entities: []querygraph.Entity{{
				ID: "entity-1", Version: 3, Title: "Alpha", Aliases: []string{},
			}},
			Relations: []querygraph.Relation{{
				ID: "relation-1", Version: 2, SourceEntityID: "entity-1", TargetEntityID: "entity-1", Type: "related_to",
			}},
			MatchedEntities: 1, MatchedRelations: 1,
		}, nil
	}
	graphs.neighborhood = func(
		_ context.Context,
		request querygraph.NeighborhoodRequest,
	) (querygraph.Neighborhood, error) {
		if request.EntityID != "entity-1" || request.RelationLimit != 8 {
			t.Fatalf("Neighborhood request = %#v", request)
		}
		entity := querygraph.Entity{ID: "entity-1", Version: 3, Aliases: []string{}}
		return querygraph.Neighborhood{
			Center: entity, Entities: []querygraph.Entity{entity},
			Relations: []querygraph.Relation{}, MatchedRelations: 0,
		}, nil
	}
	graphs.browseCommunities = func(
		_ context.Context,
		request querygraph.CommunityRequest,
	) (querygraph.CommunityPage, error) {
		if request.Query != "" || request.ParentID == nil || *request.ParentID != "community-root" ||
			request.Page != 2 || request.PageSize != 5 {
			t.Fatalf("Community request = %#v", request)
		}
		parent := "community-root"
		return querygraph.CommunityPage{
			StructureID: "structure-1", CommunitySetID: "communities-1", CorporaID: "corpus-1",
			Page: 2, PageSize: 5, Total: 6,
			Communities: []querygraph.Community{{
				ID: "community-1", ParentID: &parent,
			}},
		}, nil
	}
	dependencies.graphs = graphs
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)

	entityResponse := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities?query=alpha&entity_type=person&limit=12", nil,
	)
	var entityBody httpEntityGraph
	decodeHTTPTestResponse(t, entityResponse, &entityBody)
	if entityResponse.Code != http.StatusOK || len(entityBody.Entities) != 1 || entityBody.Entities[0].Version != 3 ||
		entityBody.Entities[0].Aliases == nil || entityBody.Relations == nil {
		t.Fatalf("Entity response = %d/%#v", entityResponse.Code, entityBody)
	}

	neighborResponse := serveHTTPRequest(
		t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities/entity-1/neighbors?limit=8", nil,
	)
	var neighborBody httpEntityNeighborhood
	decodeHTTPTestResponse(t, neighborResponse, &neighborBody)
	if neighborResponse.Code != http.StatusOK || neighborBody.Center.ID != "entity-1" ||
		neighborBody.Entities == nil || neighborBody.Relations == nil {
		t.Fatalf("Neighborhood response = %d/%#v", neighborResponse.Code, neighborBody)
	}

	communityResponse := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/communities?parent_id=community-root&page=2&page_size=5", nil,
	)
	var communityBody httpCommunityGraph
	decodeHTTPTestResponse(t, communityResponse, &communityBody)
	if communityResponse.Code != http.StatusOK || communityBody.Total != 6 ||
		len(communityBody.Communities) != 1 || communityBody.Communities[0].ParentID == nil ||
		*communityBody.Communities[0].ParentID != "community-root" {
		t.Fatalf("Community response = %d/%#v", communityResponse.Code, communityBody)
	}
}

func TestHTTPGraphEndpointsRejectInvalidInputAndMapMissingRecords(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	graphs := &testGraphBrowser{}
	graphs.neighborhood = func(
		context.Context,
		querygraph.NeighborhoodRequest,
	) (querygraph.Neighborhood, error) {
		return querygraph.Neighborhood{}, querygraph.ErrEntityNotFound
	}
	graphs.browseCommunities = func(
		context.Context,
		querygraph.CommunityRequest,
	) (querygraph.CommunityPage, error) {
		return querygraph.CommunityPage{}, querygraph.ErrCommunityNotFound
	}
	graphs.browseEntities = func(
		context.Context,
		querygraph.EntityGraphRequest,
	) (querygraph.EntityGraph, error) {
		return querygraph.EntityGraph{}, nil
	}
	dependencies.graphs = graphs
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)

	invalid := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities?limit=nope", nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d", invalid.Code)
	}
	missingEntity := serveHTTPRequest(t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities/missing/neighbors", nil)
	var entityFailure httpErrorEnvelope
	decodeHTTPTestResponse(t, missingEntity, &entityFailure)
	if missingEntity.Code != http.StatusNotFound || entityFailure.Error.Code != "graph_entity_not_found" {
		t.Fatalf("missing Entity = %d/%#v", missingEntity.Code, entityFailure)
	}
	missingCommunity := serveHTTPRequest(
		t, handler, http.MethodGet, "/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/communities?parent_id=missing", nil,
	)
	var communityFailure httpErrorEnvelope
	decodeHTTPTestResponse(t, missingCommunity, &communityFailure)
	if missingCommunity.Code != http.StatusNotFound || communityFailure.Error.Code != "graph_community_not_found" {
		t.Fatalf("missing Community = %d/%#v", missingCommunity.Code, communityFailure)
	}
}

type testGraphBrowser struct {
	browseEntities    func(context.Context, querygraph.EntityGraphRequest) (querygraph.EntityGraph, error)
	neighborhood      func(context.Context, querygraph.NeighborhoodRequest) (querygraph.Neighborhood, error)
	browseCommunities func(context.Context, querygraph.CommunityRequest) (querygraph.CommunityPage, error)
}

func (b *testGraphBrowser) BrowseEntities(
	ctx context.Context,
	request querygraph.EntityGraphRequest,
) (querygraph.EntityGraph, error) {
	return b.browseEntities(ctx, request)
}

func (b *testGraphBrowser) Neighborhood(
	ctx context.Context,
	request querygraph.NeighborhoodRequest,
) (querygraph.Neighborhood, error) {
	return b.neighborhood(ctx, request)
}

func (b *testGraphBrowser) BrowseCommunities(
	ctx context.Context,
	request querygraph.CommunityRequest,
) (querygraph.CommunityPage, error) {
	return b.browseCommunities(ctx, request)
}
