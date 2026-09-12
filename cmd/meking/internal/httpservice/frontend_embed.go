package httpservice

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// frontendDistribution contains the versioned Vue production build so the Web
// service never depends on Node, Python, or external static files at runtime.
//
//go:embed frontend/dist
var frontendDistribution embed.FS

// frontendHandler keeps API misses separate from browser navigation while
// serving one immutable embedded build with cache policy based on asset role.
type frontendHandler struct {
	files http.Handler
	root  fs.FS
	index []byte
}

func newFrontendHandler() (http.Handler, error) {
	root, err := fs.Sub(frontendDistribution, "frontend/dist")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, err
	}
	return &frontendHandler{files: http.FileServerFS(root), root: root, index: index}, nil
}

func (h *frontendHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/api/") {
		writeHTTPErrorValue(writer, http.StatusNotFound, httpError{
			Code: "not_found", Message: "API endpoint was not found",
		})
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(request.URL.Path, "/")
	if name == "" {
		h.serveIndex(writer, request)
		return
	}
	cleaned := path.Clean("/" + name)
	if cleaned != request.URL.Path || strings.ContainsRune(name, '\x00') {
		http.NotFound(writer, request)
		return
	}
	if info, err := fs.Stat(h.root, name); err == nil && info.Mode().IsRegular() {
		if strings.HasPrefix(name, "assets/") {
			writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			writer.Header().Set("Cache-Control", "no-cache")
		}
		h.files.ServeHTTP(writer, request)
		return
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		http.Error(writer, "static asset unavailable", http.StatusInternalServerError)
		return
	}

	if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" || !acceptsHTML(request) {
		http.NotFound(writer, request)
		return
	}
	h.serveIndex(writer, request)
}

func (h *frontendHandler) serveIndex(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(writer, request, "index.html", time.Time{}, bytes.NewReader(h.index))
}

func acceptsHTML(request *http.Request) bool {
	accept := strings.ToLower(request.Header.Get("Accept"))
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml") ||
		strings.Contains(accept, "*/*")
}
