package scheduler

import (
	"sort"

	"github.com/alekpopovic/orch/pkg/types"
)

type Assignment struct {
	TaskID types.TaskID
	NodeID types.NodeID
}

type Scheduler struct{}

func New() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Schedule(tasks []types.Task, nodes []types.Node) []Assignment {
	orderedTasks := append([]types.Task(nil), tasks...)
	orderedNodes := append([]types.Node(nil), nodes...)

	sort.Slice(orderedTasks, func(i, j int) bool {
		return orderedTasks[i].ID < orderedTasks[j].ID
	})
	sort.Slice(orderedNodes, func(i, j int) bool {
		return orderedNodes[i].ID < orderedNodes[j].ID
	})

	if len(orderedNodes) == 0 {
		return nil
	}

	assignments := make([]Assignment, 0, len(orderedTasks))
	for i, task := range orderedTasks {
		assignments = append(assignments, Assignment{
			TaskID: task.ID,
			NodeID: orderedNodes[i%len(orderedNodes)].ID,
		})
	}
	return assignments
}
