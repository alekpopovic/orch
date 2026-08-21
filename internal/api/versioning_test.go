package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestV1RoutingAndVersionMetadata(t *testing.T) {
	handler := newTestHandler()
	rec := doRequest(t, handler, http.MethodGet, "/v1/version", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected v1 version status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("API-Version") != "v1" {
		t.Fatalf("expected API-Version v1, got %q", rec.Header().Get("API-Version"))
	}
	var version APIVersionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &version); err != nil {
		t.Fatal(err)
	}
	if version.ServerVersion != "0.3.0" || version.DatabaseSchemaVersion != 16 || version.MinimumAgentVersion == "" {
		t.Fatalf("unexpected compatibility metadata: %#v", version)
	}

	missing := doRequest(t, handler, http.MethodGet, "/v2/services", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected v2 to remain unavailable, got %d", missing.Code)
	}
}

func TestDeprecatedEndpointHeaders(t *testing.T) {
	sunset := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	handler := deprecated(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), Deprecation{Sunset: sunset, Link: "https://example.test/migration"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/old", nil))
	if rec.Header().Get("Deprecation") != "true" || rec.Header().Get("Sunset") == "" || rec.Header().Get("Link") == "" {
		t.Fatalf("missing deprecation metadata: %#v", rec.Header())
	}
}
