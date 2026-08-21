package scheduler

import (
	"fmt"
	"github.com/alekpopovic/orch/pkg/types"
	"math/rand"
	"reflect"
	"testing"
)

func TestSchedulerRandomizedInvariants(t *testing.T) {
	for _, seed := range []int64{1, 7, 42, 99, 20260821} {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			nodes := make([]types.Node, 5+rng.Intn(10))
			for i := range nodes {
				status := types.NodeReady
				if rng.Intn(4) == 0 {
					status = []types.NodeStatus{types.NodeDraining, types.NodeOffline}[rng.Intn(2)]
				}
				nodes[i] = types.Node{ID: types.NodeID(fmt.Sprintf("n-%02d", i)), Status: status, Labels: map[string]string{"zone": []string{"a", "b"}[rng.Intn(2)]}, Allocatable: types.Resources{CPU: int64(1000 + rng.Intn(4000)), Memory: int64(1+rng.Intn(8)) << 30}}
			}
			services := map[types.ServiceID]types.Service{}
			tasks := []types.Task{}
			for i := 0; i < 100; i++ {
				id := types.ServiceID(fmt.Sprintf("s-%03d", i))
				cpu := int64(50 + rng.Intn(500))
				memory := int64(32+rng.Intn(512)) << 20
				constraints := []types.PlacementConstraint{}
				if rng.Intn(2) == 0 {
					constraints = append(constraints, types.PlacementConstraint{Key: "zone", Operator: types.ConstraintEquals, Value: []string{"a", "b"}[rng.Intn(2)]})
				}
				port := 0
				if rng.Intn(4) == 0 {
					port = 30000 + rng.Intn(20)
				}
				services[id] = types.Service{ID: id, Status: types.ServiceActive, Spec: types.ServiceSpec{Name: string(id), Image: "nginx", ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: cpu, Memory: memory}}, PlacementConstraints: constraints, Ports: func() []types.Port {
					if port == 0 {
						return nil
					}
					return []types.Port{{Protocol: types.PortTCP, ContainerPort: 80, PublishedPort: port}}
				}()}}
				tasks = append(tasks, types.Task{ID: types.TaskID(fmt.Sprintf("t-%03d", i)), ServiceID: id, ActualStatus: types.TaskPending})
			}
			input := PlanInput{PendingTasks: tasks, Nodes: nodes, Services: services}
			first := Plan(input)
			second := Plan(input)
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("seed %d: nondeterministic", seed)
			}
			nodeByID := map[types.NodeID]types.Node{}
			for _, n := range nodes {
				nodeByID[n.ID] = n
			}
			used := map[types.NodeID]types.Resources{}
			ports := map[types.NodeID]map[int]struct{}{}
			seen := map[types.TaskID]struct{}{}
			for _, a := range first {
				if _, ok := seen[a.TaskID]; ok {
					t.Fatalf("seed %d duplicate task %s", seed, a.TaskID)
				}
				seen[a.TaskID] = struct{}{}
				node := nodeByID[a.NodeID]
				if node.Status != types.NodeReady {
					t.Fatalf("seed %d assigned to %s", seed, node.Status)
				}
				service := services[taskService(tasks, a.TaskID)]
				if !placementMatches(service.Spec.PlacementConstraints, node.Labels) {
					t.Fatalf("seed %d placement mismatch", seed)
				}
				value := used[node.ID]
				value.CPU += service.Spec.ResourceRequirements.Requests.CPU
				value.Memory += service.Spec.ResourceRequirements.Requests.Memory
				used[node.ID] = value
				if value.CPU > node.Allocatable.CPU || value.Memory > node.Allocatable.Memory {
					t.Fatalf("seed %d capacity exceeded", seed)
				}
				if ports[node.ID] == nil {
					ports[node.ID] = map[int]struct{}{}
				}
				for _, p := range a.Ports {
					if _, ok := ports[node.ID][p.PublishedPort]; ok {
						t.Fatalf("seed %d port conflict", seed)
					}
					ports[node.ID][p.PublishedPort] = struct{}{}
				}
			}
		})
	}
}
func taskService(tasks []types.Task, id types.TaskID) types.ServiceID {
	for _, task := range tasks {
		if task.ID == id {
			return task.ServiceID
		}
	}
	return ""
}
