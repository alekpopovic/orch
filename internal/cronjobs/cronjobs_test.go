package cronjobs

import (
	"context"
	"testing"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeStore struct {
	cron []types.CronJob
	jobs []types.Job
}

func (f *fakeStore) ListCronJobs(context.Context) ([]types.CronJob, error) {
	return append([]types.CronJob(nil), f.cron...), nil
}
func (f *fakeStore) UpdateCronJob(_ context.Context, value types.CronJob) (types.CronJob, error) {
	f.cron[0] = value
	return value, nil
}
func (f *fakeStore) CreateJob(_ context.Context, spec types.JobSpec) (types.Job, error) {
	job := types.Job{ID: spec.Name, Spec: spec, Status: types.JobPending}
	f.jobs = append(f.jobs, job)
	return job, nil
}
func (f *fakeStore) ListJobs(context.Context) ([]types.Job, error) {
	return append([]types.Job(nil), f.jobs...), nil
}
func (f *fakeStore) DeleteJob(_ context.Context, id string) error {
	for i, job := range f.jobs {
		if job.ID == id {
			f.jobs = append(f.jobs[:i], f.jobs[i+1:]...)
			break
		}
	}
	return nil
}

func TestParseAndRunWithFakeClock(t *testing.T) {
	schedule, err := Parse("*/5 2 * * 1-5")
	if err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2026, 8, 17, 2, 1, 0, 0, time.UTC)
	if got := schedule.Next(monday); !got.Equal(time.Date(2026, 8, 17, 2, 5, 0, 0, time.UTC)) {
		t.Fatalf("next=%s", got)
	}
	created := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{cron: []types.CronJob{{ID: "cron", Namespace: "default", CreatedAt: created, Spec: types.CronJobSpec{Name: "report", Schedule: "* * * * *", Timezone: "UTC", ConcurrencyPolicy: types.ConcurrencyAllow, MissedScheduleLimit: 2, JobTemplate: types.JobSpec{Name: "report", Image: "busybox:1.36"}}}}}
	controller := New(store, fakeClock{now: created.Add(3 * time.Minute)})
	if err := controller.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.jobs) != 2 {
		t.Fatalf("missed limit not enforced: %d", len(store.jobs))
	}
	if store.cron[0].LastScheduleAt.IsZero() || store.cron[0].NextScheduleAt.IsZero() {
		t.Fatal("schedule state not persisted")
	}
}

func TestConcurrencyForbidAndReplace(t *testing.T) {
	created := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	active := types.Job{ID: "active", Status: types.JobRunning}
	base := types.CronJob{ID: "cron", Namespace: "default", CreatedAt: created, ActiveJobIDs: []string{"active"}, Spec: types.CronJobSpec{Name: "work", Schedule: "* * * * *", Timezone: "UTC", ConcurrencyPolicy: types.ConcurrencyForbid, JobTemplate: types.JobSpec{Name: "work", Image: "busybox"}}}
	store := &fakeStore{cron: []types.CronJob{base}, jobs: []types.Job{active}}
	if err := New(store, fakeClock{now: created.Add(time.Minute)}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.jobs) != 1 {
		t.Fatal("Forbid created overlapping job")
	}
	base.Spec.ConcurrencyPolicy = types.ConcurrencyReplace
	store.cron[0] = base
	if err := New(store, fakeClock{now: created.Add(time.Minute)}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.jobs) != 1 || store.jobs[0].ID == "active" {
		t.Fatal("Replace did not replace active job")
	}
}
