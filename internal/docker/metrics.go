package docker

import (
	"context"
	"time"
)

type RuntimeMetrics interface {
	IncDockerOperation(operation string)
	IncDockerOperationError(operation string)
}

type instrumentedRuntime struct {
	next    Runtime
	metrics RuntimeMetrics
}

func WithMetrics(runtime Runtime, metrics RuntimeMetrics) Runtime {
	if runtime == nil || metrics == nil {
		return runtime
	}
	return &instrumentedRuntime{next: runtime, metrics: metrics}
}

func (r *instrumentedRuntime) PullImage(ctx context.Context, image string, auth *RegistryAuth) error {
	err := r.measure("pull_image", func() error {
		return r.next.PullImage(ctx, image, auth)
	})
	return err
}

func (r *instrumentedRuntime) CreateContainer(ctx context.Context, spec ContainerSpec) (ContainerID, error) {
	var id ContainerID
	err := r.measure("create_container", func() error {
		var err error
		id, err = r.next.CreateContainer(ctx, spec)
		return err
	})
	return id, err
}

func (r *instrumentedRuntime) StartContainer(ctx context.Context, id ContainerID) error {
	return r.measure("start_container", func() error {
		return r.next.StartContainer(ctx, id)
	})
}

func (r *instrumentedRuntime) StopContainer(ctx context.Context, id ContainerID, timeout time.Duration) error {
	return r.measure("stop_container", func() error {
		return r.next.StopContainer(ctx, id, timeout)
	})
}

func (r *instrumentedRuntime) RemoveContainer(ctx context.Context, id ContainerID, force bool) error {
	return r.measure("remove_container", func() error {
		return r.next.RemoveContainer(ctx, id, force)
	})
}

func (r *instrumentedRuntime) InspectContainer(ctx context.Context, id ContainerID) (ContainerStatus, error) {
	var status ContainerStatus
	err := r.measure("inspect_container", func() error {
		var err error
		status, err = r.next.InspectContainer(ctx, id)
		return err
	})
	return status, err
}

func (r *instrumentedRuntime) ListManagedContainers(ctx context.Context, labels map[string]string) ([]ContainerStatus, error) {
	var statuses []ContainerStatus
	err := r.measure("list_managed_containers", func() error {
		var err error
		statuses, err = r.next.ListManagedContainers(ctx, labels)
		return err
	})
	return statuses, err
}

func (r *instrumentedRuntime) StreamLogs(ctx context.Context, id ContainerID, opts LogOptions) (<-chan LogLine, <-chan error) {
	r.metrics.IncDockerOperation("stream_logs")
	lines, errs := r.next.StreamLogs(ctx, id, opts)
	outErrs := make(chan error, 1)
	go func() {
		defer close(outErrs)
		for err := range errs {
			if err != nil {
				r.metrics.IncDockerOperationError("stream_logs")
				outErrs <- err
			}
		}
	}()
	return lines, outErrs
}

func (r *instrumentedRuntime) measure(operation string, call func() error) error {
	r.metrics.IncDockerOperation(operation)
	err := call()
	if err != nil {
		r.metrics.IncDockerOperationError(operation)
	}
	return err
}
