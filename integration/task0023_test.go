package integration_test

import (
	"context"
	"testing"
)

func TestTask0023ReleaseRevokeCannotRaceActiveTrainingClaim(t *testing.T) {
	e := newEnvironment(t)
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Three Dataset", "task0023-dataset")
	if err != nil {
		t.Fatal(err)
	}
	stamp := e.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO dataset_releases(id, tenant_id, dataset_id, revision, digest, status, published_at, created_at, updated_at, version) VALUES(?, ?, ?, 1, 'digest', 'published', ?, ?, ?, 1)`, "release-task0023", e.admin.TenantID, draft.ID, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO training_jobs(id, tenant_id, release_id, status, owner, lease_token, lease_expires_at, max_attempts, next_attempt_at, created_at, updated_at, version) VALUES(?, ?, ?, 'running', ?, 'token', ?, 3, ?, ?, ?, 1)`, "job-task0023", e.admin.TenantID, "release-task0023", e.worker.UserID, stamp, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.datasets.Revoke(context.Background(), e.steward, "release-task0023", "task0023-revoke", "active training must finish"); err == nil {
		t.Fatal("release revoked while a worker owned an active job")
	}
	var status string
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT status FROM dataset_releases WHERE id = ?`, "release-task0023").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "published" {
		t.Fatalf("active-job release changed to %s", status)
	}
}
