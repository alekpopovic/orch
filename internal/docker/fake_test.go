package docker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestFakeRuntimeRecordsOperationsAndCreatesIdempotently(t *testing.T) {
	runtime := NewFakeRuntime(WithFakeContainerIDs("container-1"))

	first, err := runtime.CreateContainer(context.Background(), containerSpecFixture())
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	second, err := runtime.CreateContainer(context.Background(), containerSpecFixture())
	if err != nil {
		t.Fatalf("create container again: %v", err)
	}
	if first != second {
		t.Fatalf("expected idempotent create to return %q, got %q", first, second)
	}
	if err := runtime.StartContainer(context.Background(), first); err != nil {
		t.Fatalf("start container: %v", err)
	}
	status, err := runtime.InspectContainer(context.Background(), first)
	if err != nil {
		t.Fatalf("inspect container: %v", err)
	}
	if !status.Running || status.Labels[TaskIDLabel] != "task-1" {
		t.Fatalf("expected running managed task container, got %#v", status)
	}
	want := []string{"create:task-1", "create:task-1", "start:container-1", "inspect:container-1"}
	if !equalStringSlices(runtime.OperationStrings(), want) {
		t.Fatalf("expected operations %#v, got %#v", want, runtime.OperationStrings())
	}
}

func TestFakeRuntimeIsThreadSafeForConcurrentCreates(t *testing.T) {
	runtime := NewFakeRuntime()
	spec := containerSpecFixture()
	const workers = 16
	var wg sync.WaitGroup
	ids := make(chan ContainerID, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := runtime.CreateContainer(context.Background(), spec)
			ids <- id
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)

	var first ContainerID
	for err := range errs {
		if err != nil {
			t.Fatalf("create container: %v", err)
		}
	}
	for id := range ids {
		if first == "" {
			first = id
			continue
		}
		if id != first {
			t.Fatalf("expected all workers to receive %q, got %q", first, id)
		}
	}
	statuses, err := runtime.ListManagedContainers(context.Background(), map[string]string{TaskIDLabel: "task-1"})
	if err != nil {
		t.Fatalf("list managed containers: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one container, got %#v", statuses)
	}
}

func TestFakeRuntimeFailureInjection(t *testing.T) {
	errPull := errors.New("pull failed")
	errCreate := errors.New("create failed")
	errStart := errors.New("start failed")
	errHealth := errors.New("health failed")

	if err := NewFakeRuntime(WithFakePullFailure(errPull)).PullImage(context.Background(), "nginx:1.27", nil); !errors.Is(err, errPull) {
		t.Fatalf("expected pull failure, got %v", err)
	}
	if _, err := NewFakeRuntime(WithFakeCreateFailure(errCreate)).CreateContainer(context.Background(), containerSpecFixture()); !errors.Is(err, errCreate) {
		t.Fatalf("expected create failure, got %v", err)
	}

	startRuntime := NewFakeRuntime(WithFakeStartFailure(errStart))
	id, err := startRuntime.CreateContainer(context.Background(), containerSpecFixture())
	if err != nil {
		t.Fatalf("create before start failure: %v", err)
	}
	if err := startRuntime.StartContainer(context.Background(), id); !errors.Is(err, errStart) {
		t.Fatalf("expected start failure, got %v", err)
	}

	healthRuntime := NewFakeRuntime(WithFakeHealthFailure(errHealth))
	if err := healthRuntime.CheckHealth(context.Background(), "container-1"); !errors.Is(err, errHealth) {
		t.Fatalf("expected health failure, got %v", err)
	}
}

func TestFakeRuntimeCanExitAfterStart(t *testing.T) {
	runtime := NewFakeRuntime(WithFakeExitAfterStart(7))
	id, err := runtime.CreateContainer(context.Background(), containerSpecFixture())
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	if err := runtime.StartContainer(context.Background(), id); err != nil {
		t.Fatalf("start container: %v", err)
	}
	status, err := runtime.InspectContainer(context.Background(), id)
	if err != nil {
		t.Fatalf("inspect container: %v", err)
	}
	if status.Running || status.ExitCode != 7 || status.FinishedAt.IsZero() {
		t.Fatalf("expected exited container with code 7, got %#v", status)
	}
}

func TestFakeRuntimeStreamLogsCanBlockUntilContextCancellation(t *testing.T) {
	started := make(chan struct{})
	runtime := NewFakeRuntime(WithFakeLogBlock(started))
	ctx, cancel := context.WithCancel(context.Background())
	lines, errs := runtime.StreamLogs(ctx, "container-1", LogOptions{Follow: true})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected stream logs to start")
	}
	cancel()
	for line := range lines {
		_ = line
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("expected no log error, got %v", err)
		}
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
