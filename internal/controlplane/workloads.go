package controlplane

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/gitops"
	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func (s *MemoryService) SetNotificationDispatcher(dispatcher interface {
	Notify(context.Context, types.Event) error
}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notificationDispatcher = dispatcher
}

func (s *MemoryService) MarkGitOpsManaged(ctx context.Context, id types.ServiceID, state types.GitOpsManagedState) (types.Service, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	service, ok := s.services[id]
	if !ok || !namespace.Matches(ctx, service.Namespace) {
		return types.Service{}, store.ErrNotFound
	}
	if state.DesiredSpec.Name == "" {
		state.DesiredSpec = service.Spec
	}
	state.Status = types.GitOpsInSync
	state.LastCheckedAt = s.now()
	service.GitOps = &state
	s.services[id] = service
	return service, nil
}

func (s *MemoryService) ListNodesByStatus(ctx context.Context, status types.NodeStatus) ([]types.Node, error) {
	items, err := s.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.Node, 0)
	for _, item := range items {
		if item.Status == status {
			out = append(out, item)
		}
	}
	return out, nil
}
func (s *MemoryService) ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error) {
	return s.ListTasks(ctx, TaskFilter{Status: status})
}
func (s *MemoryService) ListTasksByNode(ctx context.Context, nodeID types.NodeID) ([]types.Task, error) {
	return s.ListTasks(ctx, TaskFilter{NodeID: nodeID})
}

func (s *MemoryService) GitOpsStatus(ctx context.Context) ([]types.Service, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]types.Service, 0)
	for id, service := range s.services {
		if service.GitOps == nil || !namespace.Matches(ctx, service.Namespace) {
			continue
		}
		state := *service.GitOps
		state.LastCheckedAt = s.now()
		source, sourceExists := s.gitops[state.SourceID]
		if !sourceExists {
			state.Status = types.GitOpsUnknown
		} else if source.LastError != "" || state.LastError != "" {
			state.Status = types.GitOpsSyncError
		} else if reflect.DeepEqual(service.Spec, state.DesiredSpec) {
			state.Status = types.GitOpsInSync
		} else {
			state.Status = types.GitOpsDrifted
		}
		if state.Status == types.GitOpsDrifted {
			s.appendEventLocked(gitops.EventDriftDetected, types.EventWarning, "gitops", "GitOps-managed service drifted", "service", string(service.ID), state.LastCheckedAt)
			if state.Policy == types.GitOpsAutoRevert {
				service.Spec = state.DesiredSpec
				service.DeploymentVersion++
				service.UpdatedAt = state.LastCheckedAt
				state.Status = types.GitOpsInSync
				s.reconcileServiceTasksLocked(service, state.LastCheckedAt)
				s.appendEventLocked(gitops.EventDriftReverted, types.EventInfo, "gitops", "GitOps drift automatically reverted", "service", string(service.ID), state.LastCheckedAt)
			}
		}
		service.GitOps = &state
		s.services[id] = service
		items = append(items, service)
	}
	slices.SortFunc(items, func(a, b types.Service) int { return strings.Compare(a.Spec.Name, b.Spec.Name) })
	return items, nil
}

func (s *MemoryService) GitOpsDiff(ctx context.Context, name string) (gitops.Diff, error) {
	items, err := s.GitOpsStatus(ctx)
	if err != nil {
		return gitops.Diff{}, err
	}
	for _, service := range items {
		if service.Spec.Name == name || string(service.ID) == name {
			return gitops.Diff{Service: service.Spec.Name, Status: service.GitOps.Status, Desired: service.GitOps.DesiredSpec, Live: service.Spec}, nil
		}
	}
	return gitops.Diff{}, store.ErrNotFound
}

func (s *MemoryService) CreateJob(ctx context.Context, spec types.JobSpec) (types.Job, error) {
	if err := spec.Validate(); err != nil {
		return types.Job{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	req, err := spec.ResourceRequirements.WithDefaults(types.DefaultResourceDefaults())
	if err != nil {
		return types.Job{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	spec.ResourceRequirements = req
	ns := namespace.FromContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.namespaces[ns]; !ok {
		return types.Job{}, store.ErrNotFound
	}
	for _, job := range s.jobs {
		if job.Namespace == ns && job.Spec.Name == spec.Name && (job.Status == types.JobPending || job.Status == types.JobRunning) {
			return types.Job{}, store.ErrDuplicate
		}
	}
	now := s.now()
	id := newUUID()
	job := types.Job{ID: id, Namespace: ns, Spec: spec, Status: types.JobPending, Attempts: 1, CreatedAt: now, UpdatedAt: now}
	task := types.Task{ID: types.TaskID(newUUID()), Namespace: ns, JobID: id, DesiredStatus: types.TaskRunning, ActualStatus: types.TaskPending, Image: spec.Image, RequestedImage: spec.Image, Version: 1, Command: append([]string(nil), spec.Command...), ResourceRequirements: &spec.ResourceRequirements, PlacementConstraints: append([]types.PlacementConstraint(nil), spec.PlacementConstraints...), CreatedAt: now, UpdatedAt: now}
	resolved, err := s.resolveVolumeMountsLocked(ns, spec.VolumeClaims)
	if err != nil {
		return types.Job{}, err
	}
	task.VolumeMounts = resolved
	job.TaskIDs = []types.TaskID{task.ID}
	s.jobs[id] = job
	s.tasks[task.ID] = task
	s.appendEventLocked("job.created", types.EventInfo, "jobs", "job created", "job", id, now)
	return job, nil
}

func (s *MemoryService) AssignTask(ctx context.Context, id types.TaskID, nodeID types.NodeID, ports []types.Port, expectedUpdatedAt time.Time) (types.Task, error) {
	if err := ctx.Err(); err != nil {
		return types.Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return types.Task{}, store.ErrNotFound
	}
	if !expectedUpdatedAt.IsZero() && !task.UpdatedAt.Equal(expectedUpdatedAt) {
		return types.Task{}, store.ErrConflict
	}
	if task.ActualStatus != types.TaskPending {
		return types.Task{}, store.ErrConflict
	}
	node, ok := s.nodes[nodeID]
	if !ok || node.Status != types.NodeReady {
		return types.Task{}, store.ErrNotFound
	}
	if len(s.nodesForVolumeMountsLocked([]types.Node{node}, task.VolumeMounts)) == 0 {
		return types.Task{}, store.ErrConflict
	}
	task.NodeID = nodeID
	task.Ports = append([]types.Port(nil), ports...)
	task.ActualStatus = types.TaskAssigned
	task.UpdatedAt = s.now()
	s.tasks[id] = task
	s.attachTaskVolumesLocked(task, nodeID)
	return task, nil
}

func (s *MemoryService) updateJobForTaskLocked(task types.Task) {
	job, ok := s.jobs[task.JobID]
	if !ok {
		return
	}
	now := s.now()
	switch task.ActualStatus {
	case types.TaskRunning, types.TaskHealthy:
		job.Status = types.JobRunning
	case types.TaskStopped:
		code := 0
		if task.ExitCode != nil {
			code = *task.ExitCode
		}
		job.LastExitCode = &code
		if code == 0 {
			job.Status = types.JobSucceeded
			job.CompletedAt = now
		} else {
			s.retryOrFailJobLocked(&job, task, code, now)
		}
	case types.TaskFailed:
		code := 1
		if task.ExitCode != nil {
			code = *task.ExitCode
		}
		job.LastExitCode = &code
		s.retryOrFailJobLocked(&job, task, code, now)
	}
	job.UpdatedAt = now
	s.jobs[job.ID] = job
}

func (s *MemoryService) retryOrFailJobLocked(job *types.Job, previous types.Task, code int, now time.Time) {
	if job.Attempts > job.Spec.BackoffLimit {
		job.Status = types.JobFailed
		job.CompletedAt = now
		return
	}
	job.Attempts++
	task := previous
	task.ID = types.TaskID(newUUID())
	task.NodeID = ""
	task.ContainerID = ""
	task.ActualStatus = types.TaskPending
	task.DesiredStatus = types.TaskRunning
	task.RestartCount = job.Attempts - 1
	task.ExitCode = nil
	task.FailureReason = ""
	task.CreatedAt = now
	task.UpdatedAt = now
	task.StartedAt = time.Time{}
	task.FinishedAt = time.Time{}
	s.tasks[task.ID] = task
	job.TaskIDs = append(job.TaskIDs, task.ID)
	job.Status = types.JobPending
}

func (s *MemoryService) ListJobs(ctx context.Context) ([]types.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []types.Job{}
	for _, j := range s.jobs {
		if namespace.Matches(ctx, j.Namespace) {
			out = append(out, j)
		}
	}
	slices.SortFunc(out, func(a, b types.Job) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}
func (s *MemoryService) GetJob(ctx context.Context, id string) (types.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok || !namespace.Matches(ctx, j.Namespace) {
		return types.Job{}, store.ErrNotFound
	}
	return j, nil
}
func (s *MemoryService) DeleteJob(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok || !namespace.Matches(ctx, j.Namespace) {
		return store.ErrNotFound
	}
	for _, tid := range j.TaskIDs {
		delete(s.tasks, tid)
		s.detachTaskVolumesLocked(tid)
	}
	delete(s.jobs, id)
	s.appendEventLocked("job.deleted", types.EventInfo, "jobs", "job deleted", "job", id, s.now())
	return nil
}

func (s *MemoryService) CreateCronJob(ctx context.Context, spec types.CronJobSpec) (types.CronJob, error) {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Schedule) == "" {
		return types.CronJob{}, fmt.Errorf("%w: name and schedule are required", store.ErrInvalidState)
	}
	if spec.Timezone == "" {
		spec.Timezone = "UTC"
	}
	if spec.ConcurrencyPolicy == "" {
		spec.ConcurrencyPolicy = types.ConcurrencyAllow
	}
	if spec.ConcurrencyPolicy != types.ConcurrencyAllow && spec.ConcurrencyPolicy != types.ConcurrencyForbid && spec.ConcurrencyPolicy != types.ConcurrencyReplace {
		return types.CronJob{}, fmt.Errorf("%w: invalid concurrency policy", store.ErrInvalidState)
	}
	if err := spec.JobTemplate.Validate(); err != nil {
		return types.CronJob{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	ns := namespace.FromContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	item := types.CronJob{ID: newUUID(), Namespace: ns, Spec: spec, CreatedAt: now, UpdatedAt: now}
	s.cronJobs[item.ID] = item
	return item, nil
}
func (s *MemoryService) ListCronJobs(ctx context.Context) ([]types.CronJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []types.CronJob{}
	for _, v := range s.cronJobs {
		if namespace.Matches(ctx, v.Namespace) {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b types.CronJob) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}
func (s *MemoryService) GetCronJob(ctx context.Context, id string) (types.CronJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.cronJobs[id]
	if !ok || !namespace.Matches(ctx, v.Namespace) {
		return types.CronJob{}, store.ErrNotFound
	}
	return v, nil
}
func (s *MemoryService) DeleteCronJob(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cronJobs[id]
	if !ok || !namespace.Matches(ctx, v.Namespace) {
		return store.ErrNotFound
	}
	delete(s.cronJobs, id)
	return nil
}
func (s *MemoryService) SetCronJobSuspended(ctx context.Context, id string, value bool) (types.CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cronJobs[id]
	if !ok || !namespace.Matches(ctx, v.Namespace) {
		return types.CronJob{}, store.ErrNotFound
	}
	v.Spec.Suspended = value
	v.UpdatedAt = s.now()
	s.cronJobs[id] = v
	return v, nil
}
func (s *MemoryService) UpdateCronJob(ctx context.Context, v types.CronJob) (types.CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.cronJobs[v.ID]
	if !ok || !namespace.Matches(ctx, old.Namespace) {
		return types.CronJob{}, store.ErrNotFound
	}
	v.Namespace = old.Namespace
	v.CreatedAt = old.CreatedAt
	v.UpdatedAt = s.now()
	s.cronJobs[v.ID] = v
	return v, nil
}

func (s *MemoryService) CreateVolume(ctx context.Context, v types.Volume) (types.Volume, error) {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" {
		return types.Volume{}, fmt.Errorf("%w: volume name is required", store.ErrInvalidState)
	}
	if v.Driver == "" {
		v.Driver = "local"
	}
	if v.Driver != "local" {
		return types.Volume{}, fmt.Errorf("%w: only local volumes are supported", store.ErrInvalidState)
	}
	v.Namespace = namespace.FromContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.volumes {
		if x.Namespace == v.Namespace && x.Name == v.Name {
			return types.Volume{}, store.ErrDuplicate
		}
	}
	v.ID = newUUID()
	v.CreatedAt = s.now()
	s.volumes[v.ID] = v
	return v, nil
}
func (s *MemoryService) ListVolumes(ctx context.Context) ([]types.Volume, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []types.Volume{}
	for _, v := range s.volumes {
		if namespace.Matches(ctx, v.Namespace) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *MemoryService) GetVolume(ctx context.Context, id string) (types.Volume, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.volumes[id]
	if !ok || !namespace.Matches(ctx, v.Namespace) {
		return types.Volume{}, store.ErrNotFound
	}
	return v, nil
}
func (s *MemoryService) DeleteVolume(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.volumes[id]
	if !ok || !namespace.Matches(ctx, v.Namespace) {
		return store.ErrNotFound
	}
	for _, a := range s.attachments {
		if a.VolumeID == id && a.DetachedAt.IsZero() {
			return store.ErrConflict
		}
	}
	delete(s.volumes, id)
	return nil
}
func (s *MemoryService) CreateVolumeClaim(ctx context.Context, c types.VolumeClaim) (types.VolumeClaim, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return types.VolumeClaim{}, fmt.Errorf("%w: claim name is required", store.ErrInvalidState)
	}
	if c.AccessMode == "" {
		c.AccessMode = types.VolumeReadWriteOnce
	}
	c.Namespace = namespace.FromContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.VolumeID != "" {
		v, ok := s.volumes[c.VolumeID]
		if !ok || v.Namespace != c.Namespace {
			return types.VolumeClaim{}, store.ErrNotFound
		}
	}
	c.ID = newUUID()
	c.CreatedAt = s.now()
	s.volumeClaims[namespacedKey(c.Namespace, c.Name)] = c
	return c, nil
}
func (s *MemoryService) ListVolumeClaims(ctx context.Context) ([]types.VolumeClaim, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []types.VolumeClaim{}
	for _, v := range s.volumeClaims {
		if namespace.Matches(ctx, v.Namespace) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *MemoryService) resolveVolumeMountsLocked(ns string, mounts []types.VolumeClaimMount) ([]types.ResolvedVolumeMount, error) {
	out := make([]types.ResolvedVolumeMount, 0, len(mounts))
	for _, m := range mounts {
		c, ok := s.volumeClaims[namespacedKey(ns, m.Claim)]
		if !ok {
			return nil, fmt.Errorf("%w: volume claim %q not found", store.ErrInvalidState, m.Claim)
		}
		if c.VolumeID == "" {
			for id, v := range s.volumes {
				if v.Namespace == ns && v.Name == c.Name {
					c.VolumeID = id
					break
				}
			}
			if c.VolumeID == "" {
				v := types.Volume{ID: newUUID(), Namespace: ns, Name: c.Name, Driver: "local", CreatedAt: s.now()}
				s.volumes[v.ID] = v
				c.VolumeID = v.ID
				s.volumeClaims[namespacedKey(ns, c.Name)] = c
			}
		}
		v := s.volumes[c.VolumeID]
		out = append(out, types.ResolvedVolumeMount{ClaimID: c.ID, VolumeID: v.ID, VolumeName: v.Name, NodeID: v.NodeID, Target: m.Target, ReadOnly: m.ReadOnly, AccessMode: c.AccessMode, AllowConcurrentWriters: c.AllowConcurrentWriters})
	}
	return out, nil
}
func (s *MemoryService) detachTaskVolumesLocked(tid types.TaskID) {
	now := s.now()
	for id, a := range s.attachments {
		if a.TaskID == tid && a.DetachedAt.IsZero() {
			a.DetachedAt = now
			s.attachments[id] = a
		}
	}
}
func (s *MemoryService) nodesForVolumeMountsLocked(nodes []types.Node, mounts []types.ResolvedVolumeMount) []types.Node {
	out := []types.Node{}
	for _, n := range nodes {
		ok := true
		for _, m := range mounts {
			if m.NodeID != "" && m.NodeID != n.ID {
				ok = false
				break
			}
			if !m.ReadOnly && m.AccessMode == types.VolumeReadWriteOnce && !m.AllowConcurrentWriters {
				for _, a := range s.attachments {
					if a.VolumeID == m.VolumeID && a.DetachedAt.IsZero() {
						ok = false
						break
					}
				}
			}
		}
		if ok {
			out = append(out, n)
		}
	}
	return out
}
func (s *MemoryService) attachTaskVolumesLocked(task types.Task, node types.NodeID) {
	for _, m := range task.VolumeMounts {
		v := s.volumes[m.VolumeID]
		if v.NodeID == "" {
			v.NodeID = node
			s.volumes[v.ID] = v
		}
		id := newUUID()
		s.attachments[id] = types.VolumeAttachment{ID: id, VolumeID: m.VolumeID, TaskID: task.ID, NodeID: node, ReadOnly: m.ReadOnly, AttachedAt: s.now()}
	}
}

func (s *MemoryService) CreateNotificationSink(ctx context.Context, v types.NotificationSink) (types.NotificationSink, error) {
	if err := v.Validate(); err != nil {
		return types.NotificationSink{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	v.Namespace = namespace.FromContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	v.ID = newUUID()
	v.CreatedAt = s.now()
	s.notificationSinks[v.ID] = v
	return v, nil
}
func (s *MemoryService) ListNotificationSinks(ctx context.Context) ([]types.NotificationSink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []types.NotificationSink{}
	for _, v := range s.notificationSinks {
		if namespace.Matches(ctx, v.Namespace) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *MemoryService) GetNotificationSink(ctx context.Context, id string) (types.NotificationSink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.notificationSinks[id]
	if !ok || !namespace.Matches(ctx, v.Namespace) {
		return types.NotificationSink{}, store.ErrNotFound
	}
	return v, nil
}
func (s *MemoryService) DeleteNotificationSink(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.notificationSinks[id]
	if !ok || !namespace.Matches(ctx, v.Namespace) {
		return store.ErrNotFound
	}
	delete(s.notificationSinks, id)
	return nil
}
