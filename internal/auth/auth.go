package auth

import "context"

type Principal struct {
	Subject string
}

type Authorizer interface {
	Authorize(ctx context.Context, principal Principal, action string, resource string) error
}
