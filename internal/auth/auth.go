package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type Principal struct {
	Subject        string
	Role           Role
	NamespaceRoles map[string]Role
}

type contextKey struct{}

var (
	ErrMissingToken = errors.New("missing bearer token")
	ErrInvalidToken = errors.New("invalid bearer token")
)

type Claims struct {
	Subject        string          `json:"sub"`
	Role           Role            `json:"role,omitempty"`
	NamespaceRoles map[string]Role `json:"namespace_roles,omitempty"`
	ExpiresAt      int64           `json:"exp,omitempty"`
	IssuedAt       int64           `json:"iat,omitempty"`
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func ParseBearer(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", ErrMissingToken
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", ErrInvalidToken
	}
	return strings.TrimSpace(parts[1]), nil
}

func SignJWT(claims Claims, secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("JWT secret is required")
	}
	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := sign(unsigned, secret)
	return unsigned + "." + signature, nil
}

func ValidateJWT(token string, secret string, now time.Time) (Principal, error) {
	if strings.TrimSpace(secret) == "" {
		return Principal{}, fmt.Errorf("JWT secret is required")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, ErrInvalidToken
	}
	unsigned := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(sign(unsigned, secret))) {
		return Principal{}, ErrInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if header.Algorithm != "HS256" {
		return Principal{}, ErrInvalidToken
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if strings.TrimSpace(claims.Subject) == "" || (!ValidRole(claims.Role) && len(claims.NamespaceRoles) == 0) {
		return Principal{}, ErrInvalidToken
	}
	for namespace, role := range claims.NamespaceRoles {
		if strings.TrimSpace(namespace) == "" || !ValidRole(role) {
			return Principal{}, ErrInvalidToken
		}
	}
	if claims.ExpiresAt > 0 && !now.Before(time.Unix(claims.ExpiresAt, 0)) {
		return Principal{}, ErrInvalidToken
	}
	return Principal{Subject: claims.Subject, Role: claims.Role, NamespaceRoles: claims.NamespaceRoles}, nil
}

func (principal Principal) RoleForNamespace(namespace string) (Role, bool) {
	if ValidRole(principal.Role) {
		return principal.Role, true
	}
	role, ok := principal.NamespaceRoles[strings.ToLower(strings.TrimSpace(namespace))]
	return role, ok && ValidRole(role)
}

func ValidRole(role Role) bool {
	switch role {
	case RoleAdmin, RoleOperator, RoleViewer:
		return true
	default:
		return false
	}
}

func HasRole(actual Role, required Role) bool {
	return roleRank(actual) >= roleRank(required)
}

func ParseStaticUsers(raw string) (map[string]Role, error) {
	users := make(map[string]Role)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return users, nil
	}
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid user entry %q", entry)
		}
		subject := strings.TrimSpace(parts[0])
		role := Role(strings.TrimSpace(parts[1]))
		if subject == "" || !ValidRole(role) {
			return nil, fmt.Errorf("invalid user entry %q", entry)
		}
		users[subject] = role
	}
	return users, nil
}

func roleRank(role Role) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func sign(unsigned string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
