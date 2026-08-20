package httpapi

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WithStaticApp serves a built single-page application beside the API when staticDir is
// configured. API and documentation paths always keep their normal JSON/HTML behavior.
func WithStaticApp(api http.Handler, staticDir string) http.Handler {
	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		return api
	}

	files := http.FileServer(http.Dir(staticDir))
	indexPath := filepath.Join(staticDir, "index.html")
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if isAPIPath(request.URL.Path) || (request.Method != http.MethodGet && request.Method != http.MethodHead) {
			api.ServeHTTP(response, request)
			return
		}

		cleanPath := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
		if cleanPath != "." && cleanPath != "" {
			candidate := filepath.Join(staticDir, filepath.FromSlash(cleanPath))
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				files.ServeHTTP(response, request)
				return
			}
		}

		http.ServeFile(response, request, indexPath)
	})
}

func isAPIPath(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") ||
		requestPath == "/health" || strings.HasPrefix(requestPath, "/health/") ||
		requestPath == "/docs" || strings.HasPrefix(requestPath, "/docs/") ||
		requestPath == "/openapi.json"
}
