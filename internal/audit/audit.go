package audit

import (
	"context"
	"net/url"
	"strings"
	"time"
)

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
)

type Log struct {
	ID         string            `json:"id"`
	ActorType  ActorType         `json:"actor_type"`
	ActorID    string            `json:"actor_id"`
	Action     string            `json:"action"`
	TargetType string            `json:"target_type"`
	TargetID   string            `json:"target_id"`
	RequestID  string            `json:"request_id,omitempty"`
	SourceIP   string            `json:"source_ip,omitempty"`
	Outcome    Outcome           `json:"outcome"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}

type Filter struct {
	ActorType  ActorType
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Outcome    Outcome
	Since      time.Time
	Limit      int
}

type Store interface {
	AppendAuditLog(ctx context.Context, log Log) (Log, error)
	ListAuditLogs(ctx context.Context, filter Filter) ([]Log, error)
}

func Normalize(log Log, now time.Time) Log {
	log.ActorType = normalizeActorType(log.ActorType)
	log.ActorID = strings.TrimSpace(log.ActorID)
	if log.ActorID == "" {
		log.ActorID = "unknown"
	}
	log.Action = strings.TrimSpace(log.Action)
	log.TargetType = strings.TrimSpace(log.TargetType)
	log.TargetID = strings.TrimSpace(log.TargetID)
	if log.TargetID == "" {
		log.TargetID = "unknown"
	}
	log.RequestID = strings.TrimSpace(log.RequestID)
	log.SourceIP = strings.TrimSpace(log.SourceIP)
	log.Outcome = normalizeOutcome(log.Outcome)
	log.Metadata = RedactMetadata(log.Metadata)
	if log.Timestamp.IsZero() {
		log.Timestamp = now.UTC()
	} else {
		log.Timestamp = log.Timestamp.UTC()
	}
	return log
}

func RedactMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(metadata))
	for key, value := range metadata {
		redacted[key] = redactMetadataValue(key, value)
	}
	return redacted
}

func redactMetadataValue(key string, value string) string {
	if isSensitiveMetadataKey(key) {
		return "[REDACTED]"
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

func isSensitiveMetadataKey(key string) bool {
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

func normalizeActorType(actorType ActorType) ActorType {
	switch actorType {
	case ActorUser, ActorAgent, ActorSystem:
		return actorType
	default:
		return ActorSystem
	}
}

func normalizeOutcome(outcome Outcome) Outcome {
	switch outcome {
	case OutcomeSuccess, OutcomeFailure:
		return outcome
	default:
		return OutcomeFailure
	}
}
