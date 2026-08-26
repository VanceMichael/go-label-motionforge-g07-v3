package integration_test

import (
	"context"
	"testing"
)

func TestTask0028RetryPreservesCheckpointAndAttemptHistory(t *testing.T) {
	e := newEnvironment(t)
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Eight Dataset", "task0028-dataset")
	if err != nil {
		t.Fatal(err)
	}
	stamp := e.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO dataset_releases(id, tenant_id, dataset_id, revision, digest, status, published_at, created_at, updated_at, version) VALUES(?, ?, ?, 1, 'digest', 'published', ?, ?, ?, 1)`, "release-task0028", e.admin.TenantID, draft.ID, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO training_jobs(id, tenant_id, release_id, status, max_attempts, next_attempt_at, created_at, updated_at, version) VALUES(?, ?, ?, 'queued', 3, ?, ?, ?, 1)`, "job-task0028", e.admin.TenantID, "release-task0028", stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	claim, err := e.training.Claim(context.Background(), e.worker)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := e.training.Checkpoint(context.Background(), e.worker, claim.Job.ID, claim.Job.LeaseToken, "published-step", claim.Job.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.training.Fail(context.Background(), e.worker, claim.Job.ID, claim.Job.LeaseToken, "temporary upload failure", "task0028-fail", checkpoint.Version, false); err != nil {
		t.Fatal(err)
	}
	job, err := e.training.Get(context.Background(), e.worker, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Checkpoint != "published-step" {
		t.Fatalf("retry lost durable checkpoint %q", job.Checkpoint)
	}
	var attempts int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM job_attempts WHERE job_id = ?`, claim.Job.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("retry changed attempt history count to %d", attempts)
	}
}
