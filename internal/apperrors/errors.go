package apperrors

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/alekpopovic/orch/internal/store"
)

type Code string

const (
	CodeNotFound           Code = "not_found"
	CodeInvalidArgument    Code = "invalid_argument"
	CodeConflict           Code = "conflict"
	CodeQuotaExceeded      Code = "quota_exceeded"
	CodeUnauthorized       Code = "unauthorized"
	CodeForbidden          Code = "forbidden"
	CodeFailedPrecondition Code = "failed_precondition"
	CodeUnavailable        Code = "unavailable"
	CodeInternal           Code = "internal"
)

type Error struct {
	Code    Code
	Message string
	Details map[string]any
	Err     error
}

func New(code Code, message string) *Error {
	return &Error{Code: normalizeCode(code), Message: strings.TrimSpace(message)}
}

func Wrap(code Code, message string, err error) *Error {
	return &Error{Code: normalizeCode(code), Message: strings.TrimSpace(message), Err: err}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(normalizeCode(e.Code))
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) WithDetails(details map[string]any) *Error {
	if e == nil {
		return e
	}
	e.Details = RedactDetails(details)
	return e
}

func CodeOf(err error) Code {
	if err == nil {
		return CodeInternal
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return normalizeCode(appErr.Code)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		return CodeNotFound
	case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrDuplicate):
		return CodeConflict
	case errors.Is(err, store.ErrInvalidState):
		return CodeInvalidArgument
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return CodeUnavailable
	default:
		return CodeInternal
	}
}

func MessageOf(err error) string {
	if err == nil {
		return ""
	}
	code := CodeOf(err)
	if code == CodeInternal {
		return "internal server error"
	}
	return err.Error()
}

func DetailsOf(err error) map[string]any {
	var appErr *Error
	if errors.As(err, &appErr) {
		return RedactDetails(appErr.Details)
	}
	return nil
}

func RedactDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	redacted := make(map[string]any, len(details))
	for _, key := range keys {
		redacted[key] = redactValue(key, details[key])
	}
	return redacted
}

func StringDetails(details map[string]any) map[string]string {
	redacted := RedactDetails(details)
	if len(redacted) == 0 {
		return nil
	}
	keys := make([]string, 0, len(redacted))
	for key := range redacted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(redacted))
	for _, key := range keys {
		out[key] = fmt.Sprint(redacted[key])
	}
	return out
}

func normalizeCode(code Code) Code {
	switch code {
	case CodeNotFound, CodeInvalidArgument, CodeConflict, CodeQuotaExceeded, CodeUnauthorized, CodeForbidden, CodeFailedPrecondition, CodeUnavailable, CodeInternal:
		return code
	default:
		return CodeInternal
	}
}

func redactValue(key string, value any) any {
	if isSensitiveKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		return RedactDetails(typed)
	case map[string]string:
		nested := make(map[string]any, len(typed))
		for nestedKey, nestedValue := range typed {
			nested[nestedKey] = nestedValue
		}
		return RedactDetails(nested)
	case []any:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = redactValue(key, item)
		}
		return items
	case string:
		return redactString(key, typed)
	default:
		return typed
	}
}

func redactString(key string, value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	if strings.Contains(strings.ToLower(key), "url") {
		parsed, err := url.Parse(value)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" && parsed.User != nil {
			parsed.User = url.User("[REDACTED]")
			return parsed.String()
		}
	}
	return value
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	for _, marker := range []string{
		"authorization",
		"cookie",
		"credential",
		"database_url",
		"dsn",
		"key",
		"password",
		"secret",
		"token",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
