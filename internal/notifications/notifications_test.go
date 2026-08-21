package notifications

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDeliveryRetriesSignsAndRedacts(t *testing.T) {
	attempts := 0
	var body, signature string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		signature = r.Header.Get("X-Orch-Signature")
		status := http.StatusServiceUnavailable
		if attempts == 3 {
			status = http.StatusNoContent
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	sender := New(WithHTTPClient(client), WithAttempts(3), WithBackoff(0))
	sink := types.NotificationSink{Name: "ops", Type: types.NotificationWebhook, URL: "https://hooks.example.test", SigningSecret: "sign-me"}
	event := types.Event{Type: "rollout.failed", Message: "token=super-secret failed", Details: map[string]string{"password": "bad"}}
	if err := sender.Deliver(context.Background(), sink, event); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d", attempts)
	}
	if strings.Contains(body, "super-secret") || strings.Contains(body, "\"bad\"") {
		t.Fatalf("secret leaked: %s", body)
	}
	if !strings.HasPrefix(signature, "sha256=") {
		t.Fatalf("signature=%q", signature)
	}
}
func TestDeliveryFailureAndDispatcherFilter(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Status: "500", Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	sender := New(WithHTTPClient(client), WithAttempts(2), WithBackoff(0))
	sink := types.NotificationSink{ID: "sink", Name: "ops", Type: types.NotificationHTTP, URL: "https://example.test"}
	if err := sender.Deliver(context.Background(), sink, types.Event{}); err == nil {
		t.Fatal("expected delivery failure")
	}
	store := fakeSinkStore{sinks: []types.NotificationSink{sink}}
	dispatcher := NewDispatcher(store, sender)
	if err := dispatcher.Notify(context.Background(), types.Event{Type: "unrelated", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Notify(context.Background(), types.Event{Type: "node.offline"}); err == nil {
		t.Fatal("expected dispatcher failure")
	}
}

type fakeSinkStore struct{ sinks []types.NotificationSink }

func (f fakeSinkStore) ListNotificationSinks(context.Context) ([]types.NotificationSink, error) {
	return f.sinks, nil
}
