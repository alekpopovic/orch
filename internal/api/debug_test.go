package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alekpopovic/orch/internal/controlplane"
)

func TestPprofDisabledByDefault(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	response := httptest.NewRecorder()
	NewDebugHandler(false).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestPprofIsNotMountedOnPublicHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	response := httptest.NewRecorder()
	NewHandler(nil, controlplane.NewMemoryService()).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("public handler exposed pprof with status=%d", response.Code)
	}
}
func TestPprofEnabledOnDebugHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	response := httptest.NewRecorder()
	NewDebugHandler(true).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
}
