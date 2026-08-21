package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/pkg/types"
)

func BenchmarkServiceListAPI(b *testing.B) {
	benchmarkListAPI(b, "/v1/services")
}

func BenchmarkTaskListAPI(b *testing.B) {
	benchmarkListAPI(b, "/v1/tasks")
}

func benchmarkListAPI(b *testing.B, path string) {
	controlPlane := controlplane.NewMemoryService()
	if _, err := controlPlane.RegisterNode(context.Background(), controlplane.NodeRegistration{
		Name: "benchmark-node", AdvertiseAddress: "10.0.0.1:7443",
		Capacity: types.Resources{CPU: 1_000_000, Memory: 1 << 50}, Allocatable: types.Resources{CPU: 1_000_000, Memory: 1 << 50},
	}); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if _, err := controlPlane.CreateService(context.Background(), types.ServiceSpec{
			Name: fmt.Sprintf("benchmark-%04d", i), Image: "nginx:1.27", Replicas: 1,
		}); err != nil {
			b.Fatal(err)
		}
	}
	handler := NewHandler(nil, controlPlane)
	request := httptest.NewRequest(http.MethodGet, path, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			b.Fatalf("GET %s returned %d", path, recorder.Code)
		}
	}
}
