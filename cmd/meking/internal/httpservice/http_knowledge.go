package httpservice

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	queryknowledge "github.com/memoria-space/meking/query/knowledge"
	querysource "github.com/memoria-space/meking/query/source"
)

func (a *httpApplication) knowledgeEntities(writer http.ResponseWriter, request *http.Request) {
	limit, err := positiveHTTPQueryInteger(request, "limit", queryknowledge.DefaultPageSize)
	if err != nil || limit > queryknowledge.MaximumPageSize {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code:    "invalid_input",
			Message: "limit must be a positive integer no greater than " + strconv.Itoa(queryknowledge.MaximumPageSize),
		})
		return
	}
	page, err := a.dependencies.knowledge.BrowseEntities(
		request.Context(),
		queryknowledge.PageRequest{After: request.URL.Query().Get("after"), Limit: limit},
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPKnowledgeEntityPage(page))
}

func (a *httpApplication) knowledgeRelations(writer http.ResponseWriter, request *http.Request) {
	limit, err := positiveHTTPQueryInteger(request, "limit", queryknowledge.DefaultPageSize)
	if err != nil || limit > queryknowledge.MaximumPageSize {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code:    "invalid_input",
			Message: "limit must be a positive integer no greater than " + strconv.Itoa(queryknowledge.MaximumPageSize),
		})
		return
	}
	page, err := a.dependencies.knowledge.BrowseRelations(
		request.Context(),
		queryknowledge.PageRequest{After: request.URL.Query().Get("after"), Limit: limit},
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPKnowledgeRelationPage(page))
}

func (a *httpApplication) knowledgeClaims(writer http.ResponseWriter, request *http.Request) {
	limit, err := positiveHTTPQueryInteger(request, "limit", queryknowledge.DefaultPageSize)
	if err != nil || limit > queryknowledge.MaximumPageSize {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code:    "invalid_input",
			Message: "limit must be a positive integer no greater than " + strconv.Itoa(queryknowledge.MaximumPageSize),
		})
		return
	}
	page, err := a.dependencies.knowledge.BrowseClaims(
		request.Context(),
		queryknowledge.PageRequest{After: request.URL.Query().Get("after"), Limit: limit},
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPKnowledgeClaimPage(page))
}

func (a *httpApplication) knowledgeEntity(writer http.ResponseWriter, request *http.Request) {
	id := strings.TrimSpace(request.PathValue("id"))
	if id == "" {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: "Entity ID is required"})
		return
	}
	epochID, err := requiredEpochID(request)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	detail, err := a.dependencies.knowledge.Entity(request.Context(), id, epochID)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPKnowledgeEntityDetail(detail))
}

func (a *httpApplication) knowledgeRelation(writer http.ResponseWriter, request *http.Request) {
	id := strings.TrimSpace(request.PathValue("id"))
	if id == "" {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: "Relation ID is required"})
		return
	}
	epochID, err := requiredEpochID(request)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	detail, err := a.dependencies.knowledge.Relation(request.Context(), id, epochID)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPKnowledgeRelationDetail(detail))
}

func requiredEpochID(request *http.Request) (int64, error) {
	raw := request.URL.Query().Get("epoch_id")
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("epoch_id must be a positive integer")
	}
	return value, nil
}

func newHTTPKnowledgeEntityPage(page queryknowledge.EntityPage) httpKnowledgeEntityPage {
	items := make([]httpKnowledgeEntity, len(page.Items))
	for index, entity := range page.Items {
		items[index] = newHTTPKnowledgeEntity(entity)
	}
	return httpKnowledgeEntityPage{
		httpKnowledgePublication: httpKnowledgePublication{
			EpochID:   page.EpochID,
			CorporaID: page.CorporaID, ReportSetID: page.ReportSetID,
		},
		Items: items, NextAfter: page.NextAfter, HasMore: page.HasMore,
	}
}

func newHTTPKnowledgeRelationPage(page queryknowledge.RelationPage) httpKnowledgeRelationPage {
	items := make([]httpKnowledgeRelation, len(page.Items))
	for index, relation := range page.Items {
		items[index] = newHTTPKnowledgeRelation(relation)
	}
	return httpKnowledgeRelationPage{
		httpKnowledgePublication: httpKnowledgePublication{
			EpochID:   page.EpochID,
			CorporaID: page.CorporaID, ReportSetID: page.ReportSetID,
		},
		Items: items, NextAfter: page.NextAfter, HasMore: page.HasMore,
	}
}

func newHTTPKnowledgeClaimPage(page queryknowledge.ClaimPage) httpKnowledgeClaimPage {
	return httpKnowledgeClaimPage{
		httpKnowledgePublication: newHTTPKnowledgePublication(page.Publication),
		Items:                    newHTTPKnowledgeClaims(page.Items),
		NextAfter:                page.NextAfter,
		HasMore:                  page.HasMore,
	}
}

func newHTTPKnowledgeEntityDetail(detail queryknowledge.EntityDetail) httpKnowledgeEntityDetail {
	return httpKnowledgeEntityDetail{
		httpKnowledgePublication: newHTTPKnowledgePublication(detail.Publication),
		Entity:                   newHTTPKnowledgeEntity(detail.Entity),
		Relations:                newHTTPKnowledgeRelations(detail.Relations),
		Neighbors:                newHTTPKnowledgeEntities(detail.Neighbors),
		Claims:                   newHTTPKnowledgeClaims(detail.Claims),
		TextUnits:                newHTTPTextUnits(detail.TextUnits),
	}
}

func newHTTPKnowledgeRelationDetail(detail queryknowledge.RelationDetail) httpKnowledgeRelationDetail {
	return httpKnowledgeRelationDetail{
		httpKnowledgePublication: newHTTPKnowledgePublication(detail.Publication),
		Relation:                 newHTTPKnowledgeRelation(detail.Relation),
		Endpoints:                newHTTPKnowledgeEntities(detail.Endpoints),
		Claims:                   newHTTPKnowledgeClaims(detail.Claims),
		TextUnits:                newHTTPTextUnits(detail.TextUnits),
	}
}

func newHTTPKnowledgePublication(value queryknowledge.Publication) httpKnowledgePublication {
	return httpKnowledgePublication{
		EpochID:   value.EpochID,
		CorporaID: value.CorporaID, ReportSetID: value.ReportSetID,
	}
}

func newHTTPKnowledgeEntity(entity queryknowledge.Entity) httpKnowledgeEntity {
	return httpKnowledgeEntity{
		ID: entity.ID, Version: entity.Version, Title: entity.Title, Type: entity.Type,
		Aliases: nonNilHTTPList(entity.Aliases), Description: entity.Description,
		Degree: entity.Degree, EvidenceCount: entity.EvidenceCount,
	}
}

func newHTTPKnowledgeEntities(values []queryknowledge.Entity) []httpKnowledgeEntity {
	result := make([]httpKnowledgeEntity, len(values))
	for index, value := range values {
		result[index] = newHTTPKnowledgeEntity(value)
	}
	return result
}

func newHTTPKnowledgeRelation(relation queryknowledge.Relation) httpKnowledgeRelation {
	return httpKnowledgeRelation{
		ID: relation.ID, Version: relation.Version,
		SourceEntityID: relation.SourceEntityID, TargetEntityID: relation.TargetEntityID,
		Description: relation.Description, Weight: relation.Weight,
		CombinedDegree: relation.CombinedDegree, EvidenceCount: relation.EvidenceCount,
	}
}

func newHTTPKnowledgeRelations(values []queryknowledge.Relation) []httpKnowledgeRelation {
	result := make([]httpKnowledgeRelation, len(values))
	for index, value := range values {
		result[index] = newHTTPKnowledgeRelation(value)
	}
	return result
}

func newHTTPKnowledgeClaims(values []queryknowledge.Claim) []httpKnowledgeClaim {
	result := make([]httpKnowledgeClaim, len(values))
	for index, claim := range values {
		evidenceItems := make([]httpKnowledgeClaimEvidence, len(claim.Evidence))
		for evidenceIndex, evidence := range claim.Evidence {
			evidenceItems[evidenceIndex] = httpKnowledgeClaimEvidence{
				TextUnitID: evidence.TextUnitID, SubjectText: evidence.SubjectText,
				ObjectText: evidence.ObjectText, Status: evidence.Status,
				StartDate: evidence.StartDate, EndDate: evidence.EndDate,
				Description: evidence.Description, SourceText: evidence.SourceText,
			}
		}
		result[index] = httpKnowledgeClaim{
			ID: claim.ID, Version: claim.Version, SubjectID: claim.SubjectID,
			Type: claim.Type, Evidence: evidenceItems,
		}
	}
	return result
}

func newHTTPTextUnits(values []querysource.TextUnitSource) []httpTextUnit {
	result := make([]httpTextUnit, len(values))
	for index, value := range values {
		result[index] = httpTextUnit{
			CorporaID: value.CorporaID, TextUnitID: value.TextUnitID, Text: value.Text,
			TextID: value.TextID, DocumentID: value.DocumentID,
			DocumentLocation: value.DocumentLocation, TextTitle: value.TextTitle,
		}
	}
	return result
}
