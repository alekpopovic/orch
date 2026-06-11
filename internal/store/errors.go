package store

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalidState = errors.New("invalid state")
	ErrDuplicate    = errors.New("duplicate")
)
