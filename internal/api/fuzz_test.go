package api

import (
	"bytes"
	"github.com/alekpopovic/orch/internal/controlplane"
	"net/http"
	"net/http/httptest"
	"testing"
)

func FuzzCreateServiceRequest(f *testing.F) {
	for _, v := range [][]byte{[]byte(`{"spec":{"name":"api","image":"nginx:1.27","replicas":1}}`), []byte(`{}`), []byte(`{"spec":null}`)} {
		f.Add(v)
	}
	handler := NewHandler(nil, controlplane.NewMemoryService())
	f.Fuzz(func(t *testing.T, v []byte) {
		request := httptest.NewRequest(http.MethodPost, "/v1/services", bytes.NewReader(v))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code < 200 || response.Code > 599 {
			t.Fatalf("invalid status %d", response.Code)
		}
	})
}
