package httpservice

import (
	"net/http"
	"strings"

	querygraph "github.com/memoria-space/meking/query/graph"
)

func (a *httpApplication) entityGraph(writer http.ResponseWriter, request *http.Request) {
	limit, err := positiveHTTPQueryInteger(request, "limit", querygraph.DefaultEntityLimit)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	result, err := a.dependencies.graphs.BrowseEntities(
		request.Context(),
		querygraph.EntityGraphRequest{
			Query: request.URL.Query().Get("query"), EntityType: request.URL.Query().Get("entity_type"), Limit: limit,
		},
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPEntityGraph(result))
}

func (a *httpApplication) entityNeighborhood(writer http.ResponseWriter, request *http.Request) {
	limit, err := positiveHTTPQueryInteger(request, "limit", querygraph.DefaultEntityLimit)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	result, err := a.dependencies.graphs.Neighborhood(request.Context(), querygraph.NeighborhoodRequest{
		EntityID: request.PathValue("id"), RelationLimit: limit,
	})
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPEntityNeighborhood(result))
}

func (a *httpApplication) communityGraph(writer http.ResponseWriter, request *http.Request) {
	page, err := positiveHTTPQueryInteger(request, "page", 1)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	pageSize, err := positiveHTTPQueryInteger(request, "page_size", querygraph.DefaultCommunitySize)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	var parentID *string
	if values, present := request.URL.Query()["parent_id"]; present {
		value := ""
		if len(values) > 0 {
			value = strings.TrimSpace(values[0])
		}
		parentID = &value
	}
	result, err := a.dependencies.graphs.BrowseCommunities(request.Context(), querygraph.CommunityRequest{
		Query: request.URL.Query().Get("query"), ParentID: parentID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPCommunityGraph(result))
}

func newHTTPEntityGraph(result querygraph.EntityGraph) httpEntityGraph {
	return httpEntityGraph{
		Entities: projectHTTPGraphEntities(result.Entities), Relations: projectHTTPGraphRelations(result.Relations),
		MatchedEntities: result.MatchedEntities, MatchedRelations: result.MatchedRelations, Truncated: result.Truncated,
	}
}

func newHTTPEntityNeighborhood(result querygraph.Neighborhood) httpEntityNeighborhood {
	return httpEntityNeighborhood{
		Center:   projectHTTPGraphEntity(result.Center),
		Entities: projectHTTPGraphEntities(result.Entities), Relations: projectHTTPGraphRelations(result.Relations),
		MatchedRelations: result.MatchedRelations, Truncated: result.Truncated,
	}
}

func newHTTPCommunityGraph(result querygraph.CommunityPage) httpCommunityGraph {
	communities := make([]httpGraphCommunity, 0, len(result.Communities))
	for _, community := range result.Communities {
		communities = append(communities, httpGraphCommunity{
			ID: community.ID, Number: community.Number, Level: community.Level,
			ParentID: community.ParentID, ChildCount: community.ChildCount, EntityCount: community.EntityCount,
		})
	}
	return httpCommunityGraph{
		StructureID: result.StructureID, CommunitySetID: result.CommunitySetID, CorporaID: result.CorporaID,
		Page: result.Page, PageSize: result.PageSize, Total: result.Total, Communities: communities,
	}
}

func projectHTTPGraphEntities(values []querygraph.Entity) []httpGraphEntity {
	result := make([]httpGraphEntity, 0, len(values))
	for _, value := range values {
		result = append(result, projectHTTPGraphEntity(value))
	}
	return result
}

func projectHTTPGraphEntity(value querygraph.Entity) httpGraphEntity {
	return httpGraphEntity{
		ID: value.ID, Version: value.Version, Title: value.Title, Type: value.Type,
		Aliases: append([]string{}, value.Aliases...), Description: value.Description,
		Degree: value.Degree, TextUnitCount: value.TextUnitCount,
	}
}

func projectHTTPGraphRelations(values []querygraph.Relation) []httpGraphRelation {
	result := make([]httpGraphRelation, 0, len(values))
	for _, value := range values {
		result = append(result, httpGraphRelation{
			ID: value.ID, Version: value.Version,
			SourceEntityID: value.SourceEntityID, TargetEntityID: value.TargetEntityID,
			Type: value.Type, Description: value.Description, Weight: value.Weight,
			CombinedDegree: value.CombinedDegree, TextUnitCount: value.TextUnitCount,
		})
	}
	return result
}
