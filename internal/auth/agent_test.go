package auth

import (
	"context"
	"testing"
	"time"
)

func TestAgentCredentialIssuanceStoresOnlyHash(t *testing.T) {
	issuer := NewTokenAgentCredentialIssuer(time.Minute)
	credential, record, err := issuer.Issue(context.Background(), AgentIdentity{NodeID: "node-a"})
	if err != nil {
		t.Fatalf("issue credential: %v", err)
	}
	if credential.Token == "" {
		t.Fatalf("expected token")
	}
	if record.Hash == "" {
		t.Fatalf("expected stored hash")
	}
	if record.Hash == credential.Token {
		t.Fatalf("raw token must not be stored")
	}
	if record.ExpiresAt.IsZero() || credential.ExpiresAt.IsZero() {
		t.Fatalf("expected expiry")
	}
}

func TestAgentCredentialValidation(t *testing.T) {
	issuer := NewTokenAgentCredentialIssuer(time.Minute)
	credential, record, err := issuer.Issue(context.Background(), AgentIdentity{NodeID: "node-a"})
	if err != nil {
		t.Fatalf("issue credential: %v", err)
	}

	if err := issuer.Validate(context.Background(), credential.Token, record, time.Now().UTC()); err != nil {
		t.Fatalf("validate credential: %v", err)
	}
	if err := issuer.Validate(context.Background(), "wrong", record, time.Now().UTC()); err == nil {
		t.Fatalf("expected wrong token to fail")
	}
}

func TestAgentCredentialExpiration(t *testing.T) {
	issuer := NewTokenAgentCredentialIssuer(time.Minute)
	credential, record, err := issuer.Issue(context.Background(), AgentIdentity{NodeID: "node-a"})
	if err != nil {
		t.Fatalf("issue credential: %v", err)
	}

	if err := issuer.Validate(context.Background(), credential.Token, record, record.ExpiresAt.Add(time.Nanosecond)); err == nil {
		t.Fatalf("expected expired credential to fail")
	}
}

func TestAgentCredentialRevocation(t *testing.T) {
	issuer := NewTokenAgentCredentialIssuer(time.Minute)
	credential, record, err := issuer.Issue(context.Background(), AgentIdentity{NodeID: "node-a"})
	if err != nil {
		t.Fatalf("issue credential: %v", err)
	}
	record.Revoked = true

	if err := issuer.Validate(context.Background(), credential.Token, record, time.Now().UTC()); err == nil {
		t.Fatalf("expected revoked credential to fail")
	}
}
