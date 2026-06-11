package reconciler

import "context"

type DesiredState struct {
	Tasks []TaskIntent
}

type TaskIntent struct {
	ID    string
	Image string
}

type Runtime interface {
	EnsureTask(ctx context.Context, task TaskIntent) error
}

type Reconciler struct {
	runtime Runtime
}

func New(runtime Runtime) *Reconciler {
	return &Reconciler{runtime: runtime}
}

func (r *Reconciler) Reconcile(ctx context.Context, desired DesiredState) error {
	for _, task := range desired.Tasks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.runtime.EnsureTask(ctx, task); err != nil {
			return err
		}
	}
	return nil
}
