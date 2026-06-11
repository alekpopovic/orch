package scheduler

import (
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestScheduleIsDeterministic(t *testing.T) {
	scheduler := New()
	tasks := []types.Task{{ID: "task-b"}, {ID: "task-a"}}
	nodes := []types.Node{{ID: "node-b"}, {ID: "node-a"}}

	got := scheduler.Schedule(tasks, nodes)

	if len(got) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(got))
	}
	if got[0].TaskID != "task-a" || got[0].NodeID != "node-a" {
		t.Fatalf("unexpected first assignment: %#v", got[0])
	}
}
