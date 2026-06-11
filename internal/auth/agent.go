package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

type AgentCredential struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AgentCredentialRecord struct {
	Hash      string
	ExpiresAt time.Time
	Revoked   bool
}

type AgentIdentity struct {
	NodeID string
}

type AgentCredentialIssuer interface {
	Issue(ctx context.Context, identity AgentIdentity) (AgentCredential, AgentCredentialRecord, error)
	Validate(ctx context.Context, token string, record AgentCredentialRecord, now time.Time) error
}

type TokenAgentCredentialIssuer struct {
	ttl func() time.Duration
}

func NewTokenAgentCredentialIssuer(ttl time.Duration) *TokenAgentCredentialIssuer {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &TokenAgentCredentialIssuer{ttl: func() time.Duration { return ttl }}
}

func (i *TokenAgentCredentialIssuer) Issue(_ context.Context, _ AgentIdentity) (AgentCredential, AgentCredentialRecord, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return AgentCredential{}, AgentCredentialRecord{}, fmt.Errorf("generate agent credential: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := time.Now().UTC().Add(i.ttl())
	return AgentCredential{Token: token, ExpiresAt: expiresAt}, AgentCredentialRecord{
		Hash:      HashAgentToken(token),
		ExpiresAt: expiresAt,
	}, nil
}

func (i *TokenAgentCredentialIssuer) Validate(_ context.Context, token string, record AgentCredentialRecord, now time.Time) error {
	if record.Revoked {
		return ErrInvalidToken
	}
	if record.Hash == "" || token == "" || HashAgentToken(token) != record.Hash {
		return ErrInvalidToken
	}
	if !record.ExpiresAt.IsZero() && !now.UTC().Before(record.ExpiresAt.UTC()) {
		return ErrInvalidToken
	}
	return nil
}

func HashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
