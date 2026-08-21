package scheduler

import (
	"testing"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestPlanPlacesJobTask(t *testing.T) {
	req := types.ResourceRequirements{Requests: types.Resources{CPU: 250, Memory: 256 << 20}, Limits: types.Resources{CPU: 250, Memory: 256 << 20}}
	task := types.Task{ID: "job-task", JobID: "job", Namespace: "default", ActualStatus: types.TaskPending, DesiredStatus: types.TaskRunning, Image: "busybox", ResourceRequirements: &req, UpdatedAt: time.Now()}
	node := types.Node{ID: "node", Status: types.NodeReady, Allocatable: types.Resources{CPU: 1000, Memory: 1 << 30}}
	planned := Plan(PlanInput{PendingTasks: []types.Task{task}, Nodes: []types.Node{node}, Services: map[types.ServiceID]types.Service{}})
	if len(planned) != 1 || planned[0].NodeID != node.ID {
		t.Fatalf("assignments=%#v", planned)
	}
}

func TestPlanBlocksConflictingReadWriteOnceVolume(t *testing.T) {
	service := types.Service{ID: "service", Status: types.ServiceActive, Spec: types.ServiceSpec{Name: "db", Image: "postgres", ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: 100, Memory: 1 << 20}}}}
	mount := types.ResolvedVolumeMount{VolumeID: "volume", VolumeName: "data", Target: "/data", AccessMode: types.VolumeReadWriteOnce}
	running := types.Task{ID: "running", ServiceID: service.ID, NodeID: "node", ActualStatus: types.TaskRunning, DesiredStatus: types.TaskRunning, VolumeMounts: []types.ResolvedVolumeMount{mount}}
	pending := types.Task{ID: "pending", ServiceID: service.ID, ActualStatus: types.TaskPending, DesiredStatus: types.TaskRunning, VolumeMounts: []types.ResolvedVolumeMount{mount}}
	node := types.Node{ID: "node", Status: types.NodeReady, Allocatable: types.Resources{CPU: 1000, Memory: 1 << 30}}
	planned := Plan(PlanInput{PendingTasks: []types.Task{pending}, RunningTasks: []types.Task{running}, Nodes: []types.Node{node}, Services: map[types.ServiceID]types.Service{service.ID: service}})
	if len(planned) != 0 {
		t.Fatalf("conflicting writer scheduled: %#v", planned)
	}
}
