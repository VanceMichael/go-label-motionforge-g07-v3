package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

func TestTask0025StaleTrainingRenewalCannotOverwriteNewOwner(t *testing.T) {
	e := newEnvironment(t)
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Five Dataset", "task0025-dataset")
	if err != nil {
		t.Fatal(err)
	}
	now := storage.FormatTime(e.clock.Now())
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO dataset_releases(id, tenant_id, dataset_id, revision, digest, status, published_at, created_at, updated_at, version) VALUES(?, ?, ?, 1, 'digest', 'published', ?, ?, ?, 1)`, "release-task0025", e.admin.TenantID, draft.ID, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO training_jobs(id, tenant_id, release_id, status, max_attempts, next_attempt_at, created_at, updated_at, version) VALUES(?, ?, ?, 'queued', 3, ?, ?, ?, 1)`, "job-task0025", e.admin.TenantID, "release-task0025", now, now, now); err != nil {
		t.Fatal(err)
	}
	old, err := e.training.Claim(context.Background(), e.worker)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(3 * time.Minute)
	if _, err := e.recovery.RecoverExpired(context.Background(), e.admin.TenantID, e.admin.UserID, "task0025-recover"); err != nil {
		t.Fatal(err)
	}
	second := e.createPrincipal(t, "worker-twentyfive@motion.test", "Replacement Worker", domain.RoleWorker)
	if _, err := e.training.Claim(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if _, err := e.training.Renew(context.Background(), e.worker, old.Job.ID, old.Job.LeaseToken, old.Job.Version); err == nil {
		t.Fatal("stale training renewal succeeded")
	}
}
