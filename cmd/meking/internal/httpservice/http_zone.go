package httpservice

import (
	"errors"
	"net/http"
	"time"

	"github.com/memoria-space/meking/zone"
)

type httpCreateZoneRequest struct {
	ParentZoneID *string `json:"parent_zone_id"`
}

type httpZoneDefinition struct {
	ID           string    `json:"id"`
	Role         string    `json:"role"`
	ParentZoneID *string   `json:"parent_zone_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type httpZoneDefinitionList struct {
	Zones []httpZoneDefinition `json:"zones"`
}

func (application *httpApplication) createZone(writer http.ResponseWriter, request *http.Request) {
	var input httpCreateZoneRequest
	if err := decodeHTTPJSON(writer, request, &input); err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	var (
		definition zone.Definition
		err        error
	)
	if input.ParentZoneID == nil {
		definition, err = application.dependencies.zoneCatalog.CreateRoot(request.Context())
	} else {
		parentID, parseErr := zone.ParseID(*input.ParentZoneID)
		if parseErr != nil {
			writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
				Code: "invalid_parent_zone_id", Message: "parent_zone_id must be a canonical lowercase UUID v4",
			})
			return
		}
		definition, err = application.dependencies.zoneCatalog.CreateChild(request.Context(), parentID)
	}
	if err != nil {
		writeZoneHTTPError(writer, err)
		return
	}
	writeHTTPJSON(writer, http.StatusCreated, newHTTPZoneDefinition(definition))
}

func (application *httpApplication) listZones(writer http.ResponseWriter, request *http.Request) {
	definitions, err := application.dependencies.zoneCatalog.List(request.Context())
	if err != nil {
		writeZoneHTTPError(writer, err)
		return
	}
	zones := make([]httpZoneDefinition, len(definitions))
	for index, definition := range definitions {
		zones[index] = newHTTPZoneDefinition(definition)
	}
	writeHTTPJSON(writer, http.StatusOK, httpZoneDefinitionList{Zones: zones})
}

func (application *httpApplication) zoneDefinition(writer http.ResponseWriter, request *http.Request) {
	id, err := zone.ParseID(request.PathValue("zone_id"))
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code: "invalid_zone_id", Message: "zone_id must be a canonical lowercase UUID v4",
		})
		return
	}
	definition, err := application.dependencies.zones.Resolve(request.Context(), id)
	if err != nil {
		writeZoneHTTPError(writer, err)
		return
	}
	writeHTTPJSON(writer, http.StatusOK, newHTTPZoneDefinition(definition))
}

func newHTTPZoneDefinition(definition zone.Definition) httpZoneDefinition {
	result := httpZoneDefinition{
		ID: string(definition.ID), Role: string(definition.Role), CreatedAt: definition.CreatedAt,
	}
	if parentID, ok := definition.Parent(); ok {
		value := string(parentID)
		result.ParentZoneID = &value
	}
	return result
}

func requestZoneID(request *http.Request) (string, error) {
	id, err := zone.RequireID(request.Context())
	if err != nil {
		return "", err
	}
	return string(id), nil
}

func writeZoneHTTPError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, zone.ErrNotFound):
		writeHTTPErrorValue(writer, http.StatusNotFound, httpError{Code: "zone_not_found", Message: "Zone was not found"})
	case errors.Is(err, zone.ErrChildDepth):
		writeHTTPErrorValue(writer, http.StatusConflict, httpError{Code: "invalid_zone_parent", Message: "a Child Zone cannot own another Child"})
	case errors.Is(err, zone.ErrInvalidDefinition), errors.Is(err, zone.ErrParentRequired):
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_zone", Message: "Zone definition is invalid"})
	default:
		writeHTTPErrorResponse(writer, err, false)
	}
}
