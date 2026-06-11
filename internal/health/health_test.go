package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPHealthCheck(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		wantHealthy bool
	}{
		{name: "healthy", status: http.StatusOK, wantHealthy: true},
		{name: "unhealthy", status: http.StatusInternalServerError, wantHealthy: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			checker := NewChecker()
			result, err := checker.Check(context.Background(), Check{
				Type:    HTTPProbe,
				Target:  server.URL,
				Timeout: time.Second,
			})
			if err != nil {
				t.Fatalf("check http: %v", err)
			}
			if result.Healthy != tt.wantHealthy {
				t.Fatalf("expected healthy=%v, got %v", tt.wantHealthy, result.Healthy)
			}
		})
	}
}

func TestTCPHealthCheck(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	checker := NewChecker()
	healthy, err := checker.Check(context.Background(), Check{
		Type:    TCPProbe,
		Target:  listener.Addr().String(),
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("check healthy tcp: %v", err)
	}
	if !healthy.Healthy {
		t.Fatalf("expected TCP listener to be healthy: %#v", healthy)
	}

	_ = listener.Close()
	unhealthy, err := checker.Check(context.Background(), Check{
		Type:    TCPProbe,
		Target:  listener.Addr().String(),
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("check unhealthy tcp: %v", err)
	}
	if unhealthy.Healthy {
		t.Fatalf("expected closed TCP listener to be unhealthy")
	}
}

func TestNoneHealthCheck(t *testing.T) {
	result, err := NewChecker().Check(context.Background(), Check{Type: NoneProbe})
	if err != nil {
		t.Fatalf("check none: %v", err)
	}
	if !result.Healthy {
		t.Fatalf("expected none probe to be healthy")
	}
}
