package controlplane

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func defaultRetentionConfig() types.RetentionConfig {
	return types.RetentionConfig{Events: 30 * 24 * time.Hour, AuditLogs: 90 * 24 * time.Hour, CompletedTasks: 7 * 24 * time.Hour, FailedTasks: 30 * 24 * time.Hour, Rollouts: 30 * 24 * time.Hour, CompletedJobs: 7 * 24 * time.Hour, GitOpsRecords: 30 * 24 * time.Hour}
}
func (s *MemoryService) GetRetentionStatus(ctx context.Context) (types.RetentionStatus, error) {
	if err := ctx.Err(); err != nil {
		return types.RetentionStatus{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.retentionStatus, nil
}
func (s *MemoryService) PruneRetention(ctx context.Context, dry bool) (types.PruneResult, error) {
	if err := ctx.Err(); err != nil {
		return types.PruneResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	cfg := s.retentionStatus.Config
	result := types.PruneResult{DryRun: dry}
	keepEvents := s.events[:0]
	for _, v := range s.events {
		if v.Timestamp.Before(now.Add(-cfg.Events)) {
			result.Events++
			if dry {
				keepEvents = append(keepEvents, v)
			}
		} else {
			keepEvents = append(keepEvents, v)
		}
	}
	if !dry {
		s.events = keepEvents
	}
	keepAudit := s.auditLogs[:0]
	for _, v := range s.auditLogs {
		if v.Timestamp.Before(now.Add(-cfg.AuditLogs)) {
			result.AuditLogs++
			if dry {
				keepAudit = append(keepAudit, v)
			}
		} else {
			keepAudit = append(keepAudit, v)
		}
	}
	if !dry {
		s.auditLogs = keepAudit
	}
	for id, v := range s.tasks {
		terminal := v.ActualStatus == types.TaskStopped || v.ActualStatus == types.TaskRemoved
		completedExpired := terminal && v.FinishedAt.Before(now.Add(-cfg.CompletedTasks))
		failedExpired := v.ActualStatus == types.TaskFailed && v.FinishedAt.Before(now.Add(-cfg.FailedTasks)) && s.failedTaskResolvedLocked(v)
		if completedExpired || failedExpired {
			result.Tasks++
			if !dry {
				delete(s.tasks, id)
				s.detachTaskVolumesLocked(id)
			}
		}
	}
	for id, v := range s.deployments {
		resolved := v.Status == types.DeploymentSucceeded || v.Status == types.DeploymentRolledBack
		if resolved && v.UpdatedAt.Before(now.Add(-cfg.Rollouts)) {
			result.Rollouts++
			if !dry {
				delete(s.deployments, id)
			}
		}
	}
	for id, v := range s.jobs {
		if v.Status == types.JobSucceeded && v.CompletedAt.Before(now.Add(-cfg.CompletedJobs)) {
			result.Jobs++
			if !dry {
				delete(s.jobs, id)
			}
		}
	}
	if !dry {
		s.retentionStatus.LastRunAt = now
		s.retentionStatus.LastResult = result
		log := audit.Normalize(audit.Log{Namespace: namespace.FromContext(ctx), ActorType: audit.ActorSystem, ActorID: "retention-controller", Action: "retention.prune", TargetType: "cluster", TargetID: "retention", Outcome: audit.OutcomeSuccess}, now)
		log.ID = newUUID()
		s.auditLogs = append(s.auditLogs, log)
	}
	return result, nil
}

func (s *MemoryService) failedTaskResolvedLocked(task types.Task) bool {
	if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
		return true
	}
	for _, candidate := range s.tasks {
		if candidate.ID == task.ID || candidate.ServiceID != task.ServiceID || candidate.Version < task.Version {
			continue
		}
		if candidate.ActualStatus == types.TaskRunning || candidate.ActualStatus == types.TaskHealthy {
			return true
		}
	}
	return false
}
func (s *MemoryService) CaptureUsageSnapshots(ctx context.Context) ([]types.UsageSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	out := make([]types.UsageSnapshot, 0, len(s.namespaces))
	for ns := range s.namespaces {
		usage := s.resourceUsageLocked(ns, "", nil)
		snapshot := types.UsageSnapshot{ID: newUUID(), Namespace: ns, Timestamp: now, CPUMillicores: usage.CPUMillicores, MemoryBytes: usage.MemoryBytes, Replicas: usage.Replicas, Services: usage.Services, PublicPorts: usage.PublicPorts}
		for _, claim := range s.volumeClaims {
			if claim.Namespace == ns {
				snapshot.StorageClaims++
			}
		}
		for _, task := range s.tasks {
			if task.Namespace != ns || task.StartedAt.IsZero() {
				continue
			}
			end := now
			if !task.FinishedAt.IsZero() {
				end = task.FinishedAt
			}
			if end.After(task.StartedAt) {
				snapshot.TaskRuntimeSeconds += end.Sub(task.StartedAt).Seconds()
			}
		}
		s.usageSnapshots = append(s.usageSnapshots, snapshot)
		out = append(out, snapshot)
	}
	slices.SortFunc(out, func(a, b types.UsageSnapshot) int {
		if a.Namespace < b.Namespace {
			return -1
		}
		if a.Namespace > b.Namespace {
			return 1
		}
		return 0
	})
	return out, nil
}
func (s *MemoryService) GetUsageReport(ctx context.Context, ns string, from, to time.Time) (types.UsageReport, error) {
	if err := ctx.Err(); err != nil {
		return types.UsageReport{}, err
	}
	if ns == "" {
		ns = namespace.FromContext(ctx)
	}
	if !namespace.Matches(ctx, ns) {
		return types.UsageReport{}, fmt.Errorf("%w: namespace usage", store.ErrNotFound)
	}
	ns = namespace.Normalize(ns)
	if to.IsZero() {
		to = s.now()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	report := types.UsageReport{Namespace: ns, From: from.UTC(), To: to.UTC()}
	for _, v := range s.usageSnapshots {
		if ns != "" && v.Namespace != ns {
			continue
		}
		if v.Timestamp.Before(from) || v.Timestamp.After(to) {
			continue
		}
		report.Snapshots = append(report.Snapshots, v)
		report.Totals.CPUMillicores += v.CPUMillicores
		report.Totals.MemoryBytes += v.MemoryBytes
		report.Totals.Replicas += v.Replicas
		report.Totals.Services += v.Services
		report.Totals.TaskRuntimeSeconds += v.TaskRuntimeSeconds
		report.Totals.PublicPorts += v.PublicPorts
		report.Totals.StorageClaims += v.StorageClaims
	}
	return report, nil
}
