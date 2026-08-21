package cronjobs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/pkg/types"
)

type Store interface {
	ListCronJobs(context.Context) ([]types.CronJob, error)
	UpdateCronJob(context.Context, types.CronJob) (types.CronJob, error)
	CreateJob(context.Context, types.JobSpec) (types.Job, error)
	ListJobs(context.Context) ([]types.Job, error)
	DeleteJob(context.Context, string) error
}

type Clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Controller struct {
	store    Store
	clock    Clock
	interval time.Duration
}

func New(store Store, clock Clock) *Controller {
	if clock == nil {
		clock = realClock{}
	}
	return &Controller{store: store, clock: clock, interval: time.Second}
}

func (c *Controller) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (c *Controller) RunOnce(ctx context.Context) error {
	items, err := c.store.ListCronJobs(ctx)
	if err != nil {
		return err
	}
	now := c.clock.Now().UTC()
	for _, item := range items {
		loc, err := time.LoadLocation(item.Spec.Timezone)
		if err != nil {
			return fmt.Errorf("cronjob %s timezone: %w", item.ID, err)
		}
		schedule, err := Parse(item.Spec.Schedule)
		if err != nil {
			return fmt.Errorf("cronjob %s schedule: %w", item.ID, err)
		}
		base := item.LastScheduleAt
		if base.IsZero() {
			base = item.CreatedAt
		}
		due := schedule.Between(base.In(loc), now.In(loc), item.Spec.MissedScheduleLimit)
		item.NextScheduleAt = schedule.Next(now.In(loc)).UTC()
		if item.Spec.Suspended || len(due) == 0 {
			_, err = c.store.UpdateCronJob(namespace.WithContext(ctx, item.Namespace), item)
			if err != nil {
				return err
			}
			continue
		}
		jobCtx := namespace.WithContext(ctx, item.Namespace)
		jobs, err := c.store.ListJobs(jobCtx)
		if err != nil {
			return err
		}
		active := activeJobs(item.ActiveJobIDs, jobs)
		switch item.Spec.ConcurrencyPolicy {
		case types.ConcurrencyForbid:
			if len(active) > 0 {
				due = nil
			}
		case types.ConcurrencyReplace:
			for _, id := range active {
				if err := c.store.DeleteJob(jobCtx, id); err != nil {
					return err
				}
			}
			item.ActiveJobIDs = nil
		}
		for _, at := range due {
			spec := item.Spec.JobTemplate
			spec.Name = item.Spec.Name + "-" + at.UTC().Format("20060102-150405")
			job, err := c.store.CreateJob(jobCtx, spec)
			if err != nil {
				return err
			}
			item.ActiveJobIDs = append(item.ActiveJobIDs, job.ID)
			item.LastScheduleAt = at.UTC()
		}
		_, err = c.store.UpdateCronJob(jobCtx, item)
		if err != nil {
			return err
		}
	}
	return nil
}

func activeJobs(ids []string, jobs []types.Job) []string {
	set := map[string]types.JobStatus{}
	for _, j := range jobs {
		set[j.ID] = j.Status
	}
	out := []string{}
	for _, id := range ids {
		if set[id] == types.JobPending || set[id] == types.JobRunning {
			out = append(out, id)
		}
	}
	return out
}

type Schedule struct{ fields [5]field }
type field map[int]struct{}

func Parse(value string) (Schedule, error) {
	parts := strings.Fields(value)
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("schedule must contain five fields")
	}
	bounds := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	var s Schedule
	for i, p := range parts {
		f, err := parseField(p, bounds[i][0], bounds[i][1])
		if err != nil {
			return Schedule{}, fmt.Errorf("field %d: %w", i+1, err)
		}
		s.fields[i] = f
	}
	return s, nil
}
func parseField(raw string, minimum, maximum int) (field, error) {
	out := field{}
	for _, part := range strings.Split(raw, ",") {
		step := 1
		base := part
		if x := strings.Split(part, "/"); len(x) == 2 {
			base = x[0]
			v, err := strconv.Atoi(x[1])
			if err != nil || v <= 0 {
				return nil, fmt.Errorf("invalid step")
			}
			step = v
		} else if len(x) > 2 {
			return nil, fmt.Errorf("invalid field")
		}
		lo, hi := minimum, maximum
		if base != "*" {
			r := strings.Split(base, "-")
			v, err := strconv.Atoi(r[0])
			if err != nil {
				return nil, fmt.Errorf("invalid value")
			}
			lo, hi = v, v
			if len(r) == 2 {
				hi, err = strconv.Atoi(r[1])
				if err != nil {
					return nil, fmt.Errorf("invalid range")
				}
			} else if len(r) > 2 {
				return nil, fmt.Errorf("invalid range")
			}
		}
		if lo < minimum || hi > maximum || lo > hi {
			return nil, fmt.Errorf("value outside %d-%d", minimum, maximum)
		}
		for v := lo; v <= hi; v += step {
			out[v] = struct{}{}
		}
	}
	return out, nil
}
func (s Schedule) matches(t time.Time) bool {
	vals := []int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}
	for i, v := range vals {
		if _, ok := s.fields[i][v]; !ok {
			return false
		}
	}
	return true
}
func (s Schedule) Next(after time.Time) time.Time {
	v := after.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if s.matches(v) {
			return v
		}
		v = v.Add(time.Minute)
	}
	return time.Time{}
}
func (s Schedule) Between(after, through time.Time, limit int) []time.Time {
	if limit <= 0 {
		limit = 100
	}
	out := []time.Time{}
	next := s.Next(after)
	for !next.IsZero() && !next.After(through) && len(out) < limit {
		out = append(out, next)
		next = s.Next(next)
	}
	return out
}
