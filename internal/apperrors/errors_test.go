package apperrors

import (
	"fmt"
	"testing"

	"github.com/alekpopovic/orch/internal/store"
)

func TestRedactDetails(t *testing.T) {
	details := RedactDetails(map[string]any{
		"field":        "image",
		"password":     "secret-value",
		"database_url": "postgres://user:pass@db/app",
		"nested": map[string]any{
			"token": "agent-token",
			"hint":  "safe",
		},
	})

	if details["field"] != "image" {
		t.Fatalf("expected safe detail to remain, got %#v", details["field"])
	}
	if details["password"] != "[REDACTED]" {
		t.Fatalf("expected password to be redacted, got %#v", details["password"])
	}
	if details["database_url"] != "[REDACTED]" {
		t.Fatalf("expected database URL to be redacted, got %#v", details["database_url"])
	}
	nested, ok := details["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested details, got %#v", details["nested"])
	}
	if nested["token"] != "[REDACTED]" || nested["hint"] != "safe" {
		t.Fatalf("unexpected nested redaction %#v", nested)
	}
}

func TestCodeOfLegacyStoreErrors(t *testing.T) {
	if got := CodeOf(fmt.Errorf("load service: %w", store.ErrNotFound)); got != CodeNotFound {
		t.Fatalf("expected not_found, got %q", got)
	}
	if got := CodeOf(New(CodeForbidden, "nope")); got != CodeForbidden {
		t.Fatalf("expected forbidden, got %q", got)
	}
}
