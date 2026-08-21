package types

import "time"

type MaintenanceOperation string

const (
	MaintenanceRollout     MaintenanceOperation = "rollout"
	MaintenanceRollback    MaintenanceOperation = "rollback"
	MaintenanceNodeDrain   MaintenanceOperation = "node_drain"
	MaintenanceScaleDown   MaintenanceOperation = "autoscaling_scale_down"
	MaintenanceReplacement MaintenanceOperation = "non_urgent_replacement"
)

type MaintenanceWindow struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Namespace         string                 `json:"namespace,omitempty"`
	Global            bool                   `json:"global"`
	Schedule          string                 `json:"schedule"`
	Timezone          string                 `json:"timezone"`
	Duration          time.Duration          `json:"duration"`
	AllowedOperations []MaintenanceOperation `json:"allowed_operations"`
	Enabled           bool                   `json:"enabled"`
	CreatedAt         time.Time              `json:"created_at"`
}

type VersionInfo struct {
	APIVersion                string `json:"api_version"`
	ServerVersion             string `json:"server_version"`
	MinimumAgentVersion       string `json:"minimum_agent_version"`
	MaximumTestedAgentVersion string `json:"maximum_tested_agent_version"`
	DatabaseSchemaVersion     int    `json:"database_schema_version"`
	MinimumSchemaVersion      int    `json:"minimum_schema_version"`
	MaximumSchemaVersion      int    `json:"maximum_schema_version"`
}

type RetentionConfig struct {
	Events         time.Duration `json:"events"`
	AuditLogs      time.Duration `json:"audit_logs"`
	CompletedTasks time.Duration `json:"completed_tasks"`
	FailedTasks    time.Duration `json:"failed_tasks"`
	Rollouts       time.Duration `json:"rollouts"`
	CompletedJobs  time.Duration `json:"completed_jobs"`
	GitOpsRecords  time.Duration `json:"gitops_records"`
}
type RetentionStatus struct {
	Config     RetentionConfig `json:"config"`
	LastRunAt  time.Time       `json:"last_run_at,omitempty"`
	LastResult PruneResult     `json:"last_result"`
}
type PruneResult struct {
	DryRun        bool `json:"dry_run"`
	Events        int  `json:"events"`
	AuditLogs     int  `json:"audit_logs"`
	Tasks         int  `json:"tasks"`
	Rollouts      int  `json:"rollouts"`
	Jobs          int  `json:"jobs"`
	GitOpsRecords int  `json:"gitops_records"`
}

type UsageSnapshot struct {
	ID                 string    `json:"id"`
	Namespace          string    `json:"namespace"`
	Timestamp          time.Time `json:"timestamp"`
	CPUMillicores      int64     `json:"cpu_millicores"`
	MemoryBytes        int64     `json:"memory_bytes"`
	Replicas           int       `json:"replicas"`
	Services           int       `json:"services"`
	TaskRuntimeSeconds float64   `json:"task_runtime_seconds"`
	PublicPorts        int       `json:"public_ports"`
	StorageClaims      int       `json:"storage_claims"`
}
type UsageReport struct {
	Namespace string          `json:"namespace,omitempty"`
	From      time.Time       `json:"from"`
	To        time.Time       `json:"to"`
	Snapshots []UsageSnapshot `json:"snapshots"`
	Totals    UsageSnapshot   `json:"totals"`
}
