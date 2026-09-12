package httpservice

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrontendHandlerServesEmbeddedSPAAndAssets(t *testing.T) {
	handler, err := newFrontendHandler()
	if err != nil {
		t.Fatalf("newFrontendHandler() error = %v", err)
	}

	root := serveFrontendRequest(t, handler, http.MethodGet, "/", "text/html")
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), `<div id="app"></div>`) ||
		root.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("root status/headers/body = %d/%v/%q", root.Code, root.Header(), root.Body.String())
	}

	route := serveFrontendRequest(t, handler, http.MethodGet, "/client-route", "text/html")
	if route.Code != http.StatusOK || !strings.Contains(route.Body.String(), `<div id="app"></div>`) {
		t.Fatalf("SPA route status/body = %d/%q", route.Code, route.Body.String())
	}

	assets, err := fs.Glob(frontendDistribution, "frontend/dist/assets/*.js")
	if err != nil || len(assets) == 0 {
		t.Fatalf("embedded JS assets = %v, error = %v", assets, err)
	}
	assetPath := strings.TrimPrefix(assets[0], "frontend/dist")
	asset := serveFrontendRequest(t, handler, http.MethodHead, assetPath, "*/*")
	if asset.Code != http.StatusOK || asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset status/headers = %d/%v", asset.Code, asset.Header())
	}
}

func TestFrontendHandlerDoesNotMaskMissingAPIOrAssets(t *testing.T) {
	handler, err := newFrontendHandler()
	if err != nil {
		t.Fatalf("newFrontendHandler() error = %v", err)
	}

	api := serveFrontendRequest(t, handler, http.MethodGet, "/api/v1/missing", "application/json")
	var failure httpErrorEnvelope
	if err := json.Unmarshal(api.Body.Bytes(), &failure); err != nil {
		t.Fatalf("decode API failure %q: %v", api.Body.String(), err)
	}
	if api.Code != http.StatusNotFound || failure.Error.Code != "not_found" {
		t.Fatalf("API status/failure = %d/%#v", api.Code, failure)
	}

	for _, target := range []string{"/assets/missing.js", "/missing.json", "/client-route"} {
		accept := "*/*"
		if target == "/client-route" {
			accept = "application/json"
		}
		response := serveFrontendRequest(t, handler, http.MethodGet, target, accept)
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d", target, response.Code)
		}
	}

	mutation := serveFrontendRequest(t, handler, http.MethodPost, "/client-route", "text/html")
	if mutation.Code != http.StatusMethodNotAllowed || mutation.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("mutation status/headers = %d/%v", mutation.Code, mutation.Header())
	}
}

func serveFrontendRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	target string,
	accept string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("Accept", accept)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
