package types

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type GitOpsDriftStatus string

const (
	GitOpsInSync    GitOpsDriftStatus = "in_sync"
	GitOpsDrifted   GitOpsDriftStatus = "drifted"
	GitOpsUnknown   GitOpsDriftStatus = "unknown"
	GitOpsSyncError GitOpsDriftStatus = "sync_error"
)

type GitOpsDriftPolicy string

const (
	GitOpsWarnOnly   GitOpsDriftPolicy = "warn"
	GitOpsAutoRevert GitOpsDriftPolicy = "auto_revert"
)

type GitOpsManagedState struct {
	SourceID      string            `json:"source_id"`
	SourceCommit  string            `json:"source_commit"`
	SourcePath    string            `json:"source_path"`
	Status        GitOpsDriftStatus `json:"status"`
	Policy        GitOpsDriftPolicy `json:"policy"`
	DesiredSpec   ServiceSpec       `json:"-"`
	LastCheckedAt time.Time         `json:"last_checked_at"`
	LastError     string            `json:"last_error,omitempty"`
}

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

type JobSpec struct {
	Name                 string                `json:"name" yaml:"name"`
	Image                string                `json:"image" yaml:"image"`
	Command              []string              `json:"command,omitempty" yaml:"command,omitempty"`
	BackoffLimit         int                   `json:"backoff_limit,omitempty" yaml:"backoffLimit,omitempty"`
	ResourceRequirements ResourceRequirements  `json:"resources" yaml:"resources"`
	SecretRefs           []SecretRef           `json:"secret_refs,omitempty" yaml:"secrets,omitempty"`
	PlacementConstraints []PlacementConstraint `json:"placement_constraints,omitempty" yaml:"placement,omitempty"`
	VolumeClaims         []VolumeClaimMount    `json:"volume_claims,omitempty" yaml:"volumeClaims,omitempty"`
}

func (s JobSpec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("job name is required")
	}
	if strings.TrimSpace(s.Image) == "" || !validImageReference(s.Image) {
		return fmt.Errorf("job image is invalid")
	}
	if s.BackoffLimit < 0 {
		return fmt.Errorf("backoff_limit cannot be negative")
	}
	return s.ResourceRequirements.Validate()
}

type Job struct {
	ID           string    `json:"id"`
	Namespace    string    `json:"namespace"`
	Spec         JobSpec   `json:"spec"`
	Status       JobStatus `json:"status"`
	TaskIDs      []TaskID  `json:"task_ids,omitempty"`
	Attempts     int       `json:"attempts"`
	LastExitCode *int      `json:"last_exit_code,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
}

type ConcurrencyPolicy string

const (
	ConcurrencyAllow   ConcurrencyPolicy = "Allow"
	ConcurrencyForbid  ConcurrencyPolicy = "Forbid"
	ConcurrencyReplace ConcurrencyPolicy = "Replace"
)

type CronJobSpec struct {
	Name                string            `json:"name" yaml:"name"`
	Schedule            string            `json:"schedule" yaml:"schedule"`
	Timezone            string            `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	ConcurrencyPolicy   ConcurrencyPolicy `json:"concurrency_policy,omitempty" yaml:"concurrencyPolicy,omitempty"`
	JobTemplate         JobSpec           `json:"job_template" yaml:"jobTemplate"`
	Suspended           bool              `json:"suspended,omitempty" yaml:"suspended,omitempty"`
	MissedScheduleLimit int               `json:"missed_schedule_limit,omitempty" yaml:"missedScheduleLimit,omitempty"`
}

type CronJob struct {
	ID             string      `json:"id"`
	Namespace      string      `json:"namespace"`
	Spec           CronJobSpec `json:"spec"`
	LastScheduleAt time.Time   `json:"last_schedule_at,omitempty"`
	NextScheduleAt time.Time   `json:"next_schedule_at,omitempty"`
	ActiveJobIDs   []string    `json:"active_job_ids,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type VolumeAccessMode string

const (
	VolumeReadWriteOnce VolumeAccessMode = "ReadWriteOnce"
	VolumeReadWriteMany VolumeAccessMode = "ReadWriteMany"
)

type Volume struct {
	ID        string    `json:"id"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Driver    string    `json:"driver"`
	NodeID    NodeID    `json:"node_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type VolumeClaim struct {
	ID                     string           `json:"id"`
	Namespace              string           `json:"namespace"`
	Name                   string           `json:"name"`
	VolumeID               string           `json:"volume_id,omitempty"`
	AccessMode             VolumeAccessMode `json:"access_mode"`
	AllowConcurrentWriters bool             `json:"allow_concurrent_writers,omitempty"`
	CreatedAt              time.Time        `json:"created_at"`
}

type VolumeClaimMount struct {
	Claim    string `json:"claim" yaml:"claim"`
	Target   string `json:"target" yaml:"target"`
	ReadOnly bool   `json:"read_only,omitempty" yaml:"readOnly,omitempty"`
}

type ResolvedVolumeMount struct {
	ClaimID                string           `json:"claim_id"`
	VolumeID               string           `json:"volume_id"`
	VolumeName             string           `json:"volume_name"`
	NodeID                 NodeID           `json:"node_id,omitempty"`
	Target                 string           `json:"target"`
	ReadOnly               bool             `json:"read_only,omitempty"`
	AccessMode             VolumeAccessMode `json:"access_mode"`
	AllowConcurrentWriters bool             `json:"allow_concurrent_writers,omitempty"`
}

type VolumeAttachment struct {
	ID         string    `json:"id"`
	VolumeID   string    `json:"volume_id"`
	TaskID     TaskID    `json:"task_id"`
	NodeID     NodeID    `json:"node_id"`
	ReadOnly   bool      `json:"read_only"`
	AttachedAt time.Time `json:"attached_at"`
	DetachedAt time.Time `json:"detached_at,omitempty"`
}

type NotificationSinkType string

const (
	NotificationWebhook NotificationSinkType = "webhook"
	NotificationSlack   NotificationSinkType = "slack"
	NotificationHTTP    NotificationSinkType = "http"
)

type NotificationSink struct {
	ID            string               `json:"id"`
	Namespace     string               `json:"namespace"`
	Name          string               `json:"name"`
	Type          NotificationSinkType `json:"type"`
	URL           string               `json:"url"`
	SigningSecret string               `json:"-"`
	CreatedAt     time.Time            `json:"created_at"`
}

func (s NotificationSink) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("sink name is required")
	}
	if s.Type != NotificationWebhook && s.Type != NotificationSlack && s.Type != NotificationHTTP {
		return fmt.Errorf("unsupported sink type %q", s.Type)
	}
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("sink URL must be HTTP(S)")
	}
	return nil
}
