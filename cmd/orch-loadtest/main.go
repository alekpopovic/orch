package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/pkg/types"
)

type config struct {
	nodes       int
	services    int
	replicas    int
	failureRate float64
	duration    time.Duration
	seed        int64
}

type summary struct {
	TasksCreated           int
	TasksRunning           int
	TasksFailed            int
	AverageConvergenceTime time.Duration
	SchedulerErrors        int
	ReconcilerErrors       int
}

func main() {
	cfg := parseFlags(os.Args[1:])
	if err := cfg.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := run(context.Background(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	printSummary(result)
}

func parseFlags(args []string) config {
	cfg := config{
		nodes:       5,
		services:    20,
		replicas:    3,
		failureRate: 0.05,
		duration:    10 * time.Second,
		seed:        time.Now().UnixNano(),
	}
	fs := flag.NewFlagSet("orch-loadtest", flag.ExitOnError)
	fs.IntVar(&cfg.nodes, "nodes", cfg.nodes, "number of fake nodes")
	fs.IntVar(&cfg.services, "services", cfg.services, "number of services to create")
	fs.IntVar(&cfg.replicas, "replicas", cfg.replicas, "replicas per service")
	fs.Float64Var(&cfg.failureRate, "failure-rate", cfg.failureRate, "probability of failing a task on each simulation tick")
	fs.DurationVar(&cfg.duration, "duration", cfg.duration, "simulation duration")
	fs.Int64Var(&cfg.seed, "seed", cfg.seed, "random seed")
	_ = fs.Parse(args)
	return cfg
}

func (cfg config) validate() error {
	if cfg.nodes <= 0 {
		return fmt.Errorf("nodes must be positive")
	}
	if cfg.services <= 0 {
		return fmt.Errorf("services must be positive")
	}
	if cfg.replicas < 0 {
		return fmt.Errorf("replicas cannot be negative")
	}
	if cfg.failureRate < 0 || cfg.failureRate > 1 {
		return fmt.Errorf("failure-rate must be between 0 and 1")
	}
	if cfg.duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	return nil
}

func run(ctx context.Context, cfg config) (summary, error) {
	cp := controlplane.NewMemoryService()
	rng := rand.New(rand.NewSource(cfg.seed))
	nodes, err := registerNodes(ctx, cp, cfg.nodes)
	if err != nil {
		return summary{}, err
	}
	var result summary
	convergence := make([]time.Duration, 0, cfg.services)
	for serviceIndex := 0; serviceIndex < cfg.services; serviceIndex++ {
		start := time.Now()
		service, err := cp.CreateService(ctx, types.ServiceSpec{
			Name:     fmt.Sprintf("svc-%04d", serviceIndex),
			Image:    "nginx:1.27",
			Replicas: cfg.replicas,
			ResourceRequirements: types.ResourceRequirements{
				Requests: types.Resources{CPU: 50, Memory: 64 * 1024 * 1024},
				Limits:   types.Resources{CPU: 50, Memory: 64 * 1024 * 1024},
			},
		})
		if err != nil {
			result.ReconcilerErrors++
			return result, fmt.Errorf("create service %d: %w", serviceIndex, err)
		}
		if err := markServiceTasksRunning(ctx, cp, service.ID); err != nil {
			result.ReconcilerErrors++
			return result, err
		}
		convergence = append(convergence, time.Since(start))
	}
	deadline := time.Now().Add(cfg.duration)
	for time.Now().Before(deadline) {
		if err := simulateTick(ctx, cp, nodes, rng, cfg.failureRate, &result); err != nil {
			result.ReconcilerErrors++
			return result, err
		}
		time.Sleep(100 * time.Millisecond)
	}
	tasks, err := cp.ListTasks(ctx, controlplane.TaskFilter{})
	if err != nil {
		return result, err
	}
	result.TasksCreated = len(tasks)
	for _, task := range tasks {
		switch task.ActualStatus {
		case types.TaskRunning, types.TaskHealthy:
			result.TasksRunning++
		case types.TaskFailed:
			result.TasksFailed++
		}
	}
	result.AverageConvergenceTime = averageDuration(convergence)
	return result, nil
}

func registerNodes(ctx context.Context, cp *controlplane.MemoryService, count int) ([]types.NodeID, error) {
	nodes := make([]types.NodeID, 0, count)
	for i := 0; i < count; i++ {
		registered, err := cp.RegisterNode(ctx, controlplane.NodeRegistration{
			Name:             fmt.Sprintf("node-%03d", i),
			AdvertiseAddress: fmt.Sprintf("10.0.0.%d", i+10),
			Capacity:         types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
			Allocatable:      types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
		})
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, registered.Node.ID)
	}
	return nodes, nil
}

func markServiceTasksRunning(ctx context.Context, cp *controlplane.MemoryService, serviceID types.ServiceID) error {
	tasks, err := cp.ListTasks(ctx, controlplane.TaskFilter{ServiceID: serviceID})
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.NodeID == "" || task.ActualStatus == types.TaskRunning {
			continue
		}
		if _, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{
			TaskID:      task.ID,
			NodeID:      task.NodeID,
			Status:      types.TaskRunning,
			ContainerID: "fake-" + string(task.ID),
		}); err != nil {
			return fmt.Errorf("mark task %s running: %w", task.ID, err)
		}
	}
	return nil
}

func simulateTick(ctx context.Context, cp *controlplane.MemoryService, nodes []types.NodeID, rng *rand.Rand, failureRate float64, result *summary) error {
	if len(nodes) > 0 && rng.Float64() < 0.02 {
		nodeID := nodes[rng.Intn(len(nodes))]
		shutdown := rng.Intn(2) == 0
		if _, err := cp.HeartbeatNode(ctx, controlplane.NodeHeartbeat{
			NodeID:      nodeID,
			Capacity:    types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
			Allocatable: types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
			Shutdown:    shutdown,
		}); err != nil {
			result.SchedulerErrors++
		}
	}
	tasks, err := cp.ListTasks(ctx, controlplane.TaskFilter{})
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.NodeID == "" {
			continue
		}
		if task.ActualStatus == types.TaskAssigned || task.ActualStatus == types.TaskPending {
			if _, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{
				TaskID:      task.ID,
				NodeID:      task.NodeID,
				Status:      types.TaskRunning,
				ContainerID: "fake-" + string(task.ID),
			}); err != nil {
				result.ReconcilerErrors++
			}
			continue
		}
		if task.ActualStatus == types.TaskRunning && rng.Float64() < failureRate {
			if _, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{
				TaskID:        task.ID,
				NodeID:        task.NodeID,
				Status:        types.TaskFailed,
				ContainerID:   task.ContainerID,
				FailureReason: "loadtest random failure",
			}); err != nil {
				result.ReconcilerErrors++
			}
		}
	}
	return nil
}

func averageDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	var total time.Duration
	for _, value := range values {
		total += value
	}
	return total / time.Duration(len(values))
}

func printSummary(result summary) {
	fmt.Printf("tasks_created=%d\n", result.TasksCreated)
	fmt.Printf("tasks_running=%d\n", result.TasksRunning)
	fmt.Printf("tasks_failed=%d\n", result.TasksFailed)
	fmt.Printf("average_convergence_time=%s\n", result.AverageConvergenceTime)
	fmt.Printf("scheduler_errors=%d\n", result.SchedulerErrors)
	fmt.Printf("reconciler_errors=%d\n", result.ReconcilerErrors)
}
