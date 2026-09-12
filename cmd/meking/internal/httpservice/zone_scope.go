package httpservice

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/memoria-space/meking/zone"
)

// zoneDefinitionResolver is the HTTP adapter's Zone lookup dependency.
type zoneDefinitionResolver interface {
	Resolve(ctx context.Context, id zone.ID) (zone.Definition, error)
}

// zoneScopeMiddleware resolves the route Zone and binds it to the request Context.
func zoneScopeMiddleware(
	resolver zoneDefinitionResolver,
	next http.Handler,
) (http.Handler, error) {
	if resolver == nil {
		return nil, errors.New("create HTTP Zone scope middleware: resolver is required")
	}
	if next == nil {
		return nil, errors.New("create HTTP Zone scope middleware: next Handler is required")
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		id, err := zone.ParseID(strings.TrimSpace(request.PathValue("zone_id")))
		if err != nil {
			writeZoneScopeError(writer, err)
			return
		}
		definition, err := resolver.Resolve(request.Context(), id)
		if err != nil {
			writeZoneScopeError(writer, err)
			return
		}
		zoneContext, err := zone.NewContext(request.Context(), definition.ID)
		if err != nil {
			writeZoneScopeError(writer, err)
			return
		}
		next.ServeHTTP(writer, request.WithContext(zoneContext))
	}), nil
}

func writeZoneScopeError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, zone.ErrInvalidDefinition):
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code: "invalid_zone_id", Message: "zone_id must be a canonical lowercase UUID v4",
		})
	case errors.Is(err, zone.ErrNotFound):
		writeHTTPErrorValue(writer, http.StatusNotFound, httpError{
			Code: "zone_not_found", Message: "Zone was not found",
		})
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeHTTPErrorResponse(writer, err, false)
	default:
		writeHTTPErrorValue(writer, http.StatusInternalServerError, httpError{
			Code: "zone_resolution_failed", Message: "Zone could not be resolved",
		})
	}
}
