package notifications

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

var NotifiableEvents = map[string]struct{}{
	"rollout.failed": {}, "node.offline": {}, "task.failure_threshold_exceeded": {},
	"admission.rejected": {}, "quota.exceeded": {}, "service.deleted": {},
	"secret.deleted": {}, "scheduler.unable_to_place": {},
	"node.offline.detected": {}, "task.failed": {}, "rollout.status.changed": {},
}

type Delivery struct {
	Event  types.Event `json:"event"`
	SentAt time.Time   `json:"sent_at"`
}
type Sender struct {
	client      *http.Client
	attempts    int
	baseBackoff time.Duration
	sleep       func(context.Context, time.Duration) error
	now         func() time.Time
}
type Option func(*Sender)

func WithAttempts(v int) Option {
	return func(s *Sender) {
		if v > 0 {
			s.attempts = v
		}
	}
}
func WithBackoff(v time.Duration) Option {
	return func(s *Sender) {
		if v >= 0 {
			s.baseBackoff = v
		}
	}
}
func WithHTTPClient(v *http.Client) Option {
	return func(s *Sender) {
		if v != nil {
			s.client = v
		}
	}
}
func New(opts ...Option) *Sender {
	s := &Sender{client: &http.Client{Timeout: 10 * time.Second}, attempts: 3, baseBackoff: 100 * time.Millisecond, now: func() time.Time { return time.Now().UTC() }, sleep: func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Sender) Deliver(ctx context.Context, sink types.NotificationSink, event types.Event) error {
	if err := sink.Validate(); err != nil {
		return err
	}
	event = RedactEvent(event)
	payload, err := json.Marshal(Delivery{Event: event, SentAt: s.now()})
	if err != nil {
		return err
	}
	var last error
	for attempt := 0; attempt < s.attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, sink.URL, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "orch-notifier/1")
		if sink.SigningSecret != "" {
			mac := hmac.New(sha256.New, []byte(sink.SigningSecret))
			_, _ = mac.Write(payload)
			req.Header.Set("X-Orch-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := s.client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			err = fmt.Errorf("notification endpoint returned %s", resp.Status)
		}
		last = err
		if attempt+1 < s.attempts {
			if err := s.sleep(ctx, s.baseBackoff*time.Duration(1<<attempt)); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("deliver notification after %d attempts: %w", s.attempts, last)
}

func RedactEvent(event types.Event) types.Event {
	event.Message = redact(event.Message)
	if event.Details != nil {
		details := make(map[string]string, len(event.Details))
		for k, v := range event.Details {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") {
				details[k] = "[REDACTED]"
			} else {
				details[k] = redact(v)
			}
		}
		event.Details = details
	}
	return event
}
func redact(v string) string {
	lower := strings.ToLower(v)
	for _, marker := range []string{"password=", "token=", "secret=", "authorization:"} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		end := strings.IndexAny(v[idx+len(marker):], " ,;\n\t")
		if end < 0 {
			end = len(v) - idx - len(marker)
		}
		start := idx + len(marker)
		v = v[:start] + "[REDACTED]" + v[start+end:]
		lower = strings.ToLower(v)
	}
	return v
}

type SinkStore interface {
	ListNotificationSinks(context.Context) ([]types.NotificationSink, error)
}
type Dispatcher struct {
	store  SinkStore
	sender *Sender
}

func NewDispatcher(store SinkStore, sender *Sender) *Dispatcher {
	if sender == nil {
		sender = New()
	}
	return &Dispatcher{store: store, sender: sender}
}
func (d *Dispatcher) Notify(ctx context.Context, event types.Event) error {
	if _, ok := NotifiableEvents[event.Type]; !ok {
		return nil
	}
	sinks, err := d.store.ListNotificationSinks(ctx)
	if err != nil {
		return err
	}
	var failures []string
	for _, sink := range sinks {
		if err := d.sender.Deliver(ctx, sink, event); err != nil {
			failures = append(failures, sink.ID+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("notification delivery failed: %s", strings.Join(failures, "; "))
	}
	return nil
}
