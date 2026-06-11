package reconciler

import (
	"context"
	"testing"
)

type fakeRuntime struct {
	seen []TaskIntent
}

func (f *fakeRuntime) EnsureTask(_ context.Context, task TaskIntent) error {
	f.seen = append(f.seen, task)
	return nil
}

func TestReconcileEnsuresTasksInOrder(t *testing.T) {
	runtime := &fakeRuntime{}
	reconciler := New(runtime)

	err := reconciler.Reconcile(context.Background(), DesiredState{
		Tasks: []TaskIntent{
			{ID: "task-1", Image: "nginx:latest"},
			{ID: "task-2", Image: "redis:latest"},
		},
	})
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if len(runtime.seen) != 2 {
		t.Fatalf("expected 2 reconciled tasks, got %d", len(runtime.seen))
	}
}
