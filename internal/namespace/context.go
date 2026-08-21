package namespace

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const Default = "default"

var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type contextKey struct{}

func Normalize(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return Default
	}
	return name
}

func Validate(name string) error {
	name = Normalize(name)
	if !namePattern.MatchString(name) {
		return fmt.Errorf("namespace must contain 1-63 lowercase alphanumeric characters or hyphens")
	}
	return nil
}

func WithContext(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, contextKey{}, Normalize(name))
}

func FromContext(ctx context.Context) string {
	if ctx == nil {
		return Default
	}
	name, _ := ctx.Value(contextKey{}).(string)
	return Normalize(name)
}

func Selected(ctx context.Context) (string, bool) {
	if ctx == nil {
		return Default, false
	}
	name, ok := ctx.Value(contextKey{}).(string)
	return Normalize(name), ok
}

func Matches(ctx context.Context, objectNamespace string) bool {
	selected, explicit := Selected(ctx)
	return !explicit || selected == Normalize(objectNamespace)
}
