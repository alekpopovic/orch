package types

import "time"

type NodeID string
type TaskID string
type ServiceID string

type Node struct {
	ID        NodeID            `json:"id"`
	Labels    map[string]string `json:"labels,omitempty"`
	Capacity  Resources         `json:"capacity"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type Resources struct {
	CPU    int64 `json:"cpu"`
	Memory int64 `json:"memory"`
}

type Task struct {
	ID        TaskID    `json:"id"`
	ServiceID ServiceID `json:"service_id"`
	Image     string    `json:"image"`
	NodeID    NodeID    `json:"node_id,omitempty"`
	Desired   TaskState `json:"desired"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TaskState string

const (
	TaskPending TaskState = "pending"
	TaskRunning TaskState = "running"
	TaskStopped TaskState = "stopped"
	TaskFailed  TaskState = "failed"
)
