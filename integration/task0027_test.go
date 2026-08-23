package integration_test

import (
	"context"
	"testing"
)

func TestTask0027CompletionFailureKeepsAttemptRunningJob(t *testing.T) {
	e := newEnvironment(t)
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Seven Dataset", "task0027-dataset")
	if err != nil {
		t.Fatal(err)
	}
	stamp := e.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO dataset_releases(id, tenant_id, dataset_id, revision, digest, status, published_at, created_at, updated_at, version) VALUES(?, ?, ?, 1, 'digest', 'published', ?, ?, ?, 1)`, "release-task0027", e.admin.TenantID, draft.ID, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO training_jobs(id, tenant_id, release_id, status, max_attempts, next_attempt_at, created_at, updated_at, version) VALUES(?, ?, ?, 'queued', 3, ?, ?, ?, 1)`, "job-task0027", e.admin.TenantID, "release-task0027", stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	claim, err := e.training.Claim(context.Background(), e.worker)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := e.training.Checkpoint(context.Background(), e.worker, claim.Job.ID, claim.Job.LeaseToken, "uploaded", claim.Job.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `CREATE TRIGGER task0027_fail_attempt BEFORE UPDATE ON job_attempts BEGIN SELECT RAISE(ABORT, 'attempt store unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.training.Complete(context.Background(), e.worker, claim.Job.ID, claim.Job.LeaseToken, "s3://task0027/output", "task0027-complete", checkpoint.Version); err == nil {
		t.Fatal("completion succeeded despite attempt finalization failure")
	}
	job, err := e.training.Get(context.Background(), e.worker, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(job.Status) != "running" {
		t.Fatalf("failed completion left job in %s", job.Status)
	}
}
