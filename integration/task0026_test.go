package integration_test

import (
	"context"
	"testing"
)

func TestTask0026CanceledCheckpointDoesNotAdvanceJob(t *testing.T) {
	e := newEnvironment(t)
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Six Dataset", "task0026-dataset")
	if err != nil {
		t.Fatal(err)
	}
	now := e.clock.Now()
	stamp := now.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO dataset_releases(id, tenant_id, dataset_id, revision, digest, status, published_at, created_at, updated_at, version) VALUES(?, ?, ?, 1, 'digest', 'published', ?, ?, ?, 1)`, "release-task0026", e.admin.TenantID, draft.ID, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO training_jobs(id, tenant_id, release_id, status, max_attempts, next_attempt_at, created_at, updated_at, version) VALUES(?, ?, ?, 'queued', 3, ?, ?, ?, 1)`, "job-task0026", e.admin.TenantID, "release-task0026", stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	claim, err := e.training.Claim(context.Background(), e.worker)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.training.Checkpoint(ctx, e.worker, claim.Job.ID, claim.Job.LeaseToken, "artifact-uploaded", claim.Job.Version); err == nil {
		t.Fatal("canceled checkpoint succeeded")
	}
	job, err := e.training.Get(context.Background(), e.worker, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Checkpoint != "" {
		t.Fatalf("canceled checkpoint persisted %q", job.Checkpoint)
	}
}
