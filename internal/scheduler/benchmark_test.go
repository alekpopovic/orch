package scheduler

import (
	"fmt"
	"github.com/alekpopovic/orch/pkg/types"
	"testing"
)

func BenchmarkSchedulerScoring(b *testing.B) {
	nodes := make([]types.Node, 100)
	for i := range nodes {
		nodes[i] = types.Node{ID: types.NodeID(fmt.Sprintf("node-%03d", i)), Status: types.NodeReady, Allocatable: types.Resources{CPU: 8000, Memory: 16 << 30}}
	}
	services := map[types.ServiceID]types.Service{}
	tasks := make([]types.Task, 1000)
	for i := range tasks {
		id := types.ServiceID(fmt.Sprintf("service-%04d", i))
		services[id] = types.Service{ID: id, Status: types.ServiceActive, Spec: types.ServiceSpec{Name: string(id), Image: "nginx", ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: 100, Memory: 64 << 20}}}}
		tasks[i] = types.Task{ID: types.TaskID(fmt.Sprintf("task-%04d", i)), ServiceID: id, ActualStatus: types.TaskPending}
	}
	input := PlanInput{PendingTasks: tasks, Nodes: nodes, Services: services}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Plan(input)
	}
}
