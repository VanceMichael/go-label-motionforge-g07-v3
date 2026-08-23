package integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

func TestTask0024ConcurrentTrainingClaimHasOneAttempt(t *testing.T) {
	e := newEnvironment(t)
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Four Dataset", "task0024-dataset")
	if err != nil {
		t.Fatal(err)
	}
	now := storage.FormatTime(e.clock.Now())
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO dataset_releases(id, tenant_id, dataset_id, revision, digest, status, published_at, created_at, updated_at, version) VALUES(?, ?, ?, 1, 'digest', 'published', ?, ?, ?, 1)`, "release-task0024", e.admin.TenantID, draft.ID, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO training_jobs(id, tenant_id, release_id, status, max_attempts, next_attempt_at, created_at, updated_at, version) VALUES(?, ?, ?, 'queued', 3, ?, ?, ?, 1)`, "job-task0024", e.admin.TenantID, "release-task0024", now, now, now); err != nil {
		t.Fatal(err)
	}
	second := e.createPrincipal(t, "worker-two@motion.test", "Second Worker", domain.RoleWorker)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, err := e.training.Claim(context.Background(), e.worker)
		errs <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := e.training.Claim(context.Background(), second)
		errs <- err
	}()
	close(start)
	wait.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent workers received %d claims", successes)
	}
	var attempts int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM job_attempts WHERE job_id = ?`, "job-task0024").Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("concurrent claim created %d attempts", attempts)
	}
}
