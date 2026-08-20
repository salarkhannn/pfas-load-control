package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWithStaticAppServesAssetsAndSPAFallbackWithoutMaskingAPI(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(staticDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("fieldproof-index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "assets", "app.js"), []byte("fieldproof-asset"), 0o600); err != nil {
		t.Fatal(err)
	}

	api := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTeapot)
		_, _ = response.Write([]byte("api-response"))
	})
	handler := WithStaticApp(api, staticDir)

	assertResponse(t, handler, http.MethodGet, "/judge-demo", http.StatusOK, "fieldproof-index")
	assertResponse(t, handler, http.MethodGet, "/assets/app.js", http.StatusOK, "fieldproof-asset")
	assertResponse(t, handler, http.MethodGet, "/api/v1/judge-demo/runs", http.StatusTeapot, "api-response")
	assertResponse(t, handler, http.MethodGet, "/health/ready", http.StatusTeapot, "api-response")
	assertResponse(t, handler, http.MethodPost, "/judge-demo", http.StatusTeapot, "api-response")
}

func TestWithStaticAppDisabledReturnsAPIHandler(t *testing.T) {
	api := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	assertResponse(t, WithStaticApp(api, ""), http.MethodGet, "/judge-demo", http.StatusNoContent, "")
}

func assertResponse(t *testing.T, handler http.Handler, method, requestPath string, status int, body string) {
	t.Helper()
	request := httptest.NewRequest(method, requestPath, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s status = %d, want %d", method, requestPath, response.Code, status)
	}
	if response.Body.String() != body {
		t.Fatalf("%s %s body = %q, want %q", method, requestPath, response.Body.String(), body)
	}
}
