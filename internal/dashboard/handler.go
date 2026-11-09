package dashboard

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed templates/index.html
var indexHTML []byte

//go:embed templates/style.css
var styleCSS []byte

//go:embed templates/dashboard.js
var dashboardJS []byte

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

// ServeHTTP writes the dashboard HTML, CSS, or JS payload to the client based on path.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "style.css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(styleCSS)
	case strings.HasSuffix(path, "dashboard.js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(dashboardJS)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexHTML)
	}
}
