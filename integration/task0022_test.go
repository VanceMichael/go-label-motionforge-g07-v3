package integration_test

import (
	"context"
	"testing"
)

func TestTask0022PublishFailureLeavesNoVisibleRelease(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	stamp := e.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO annotation_batches(id, tenant_id, capture_id, status, created_at, updated_at, version) VALUES(?, ?, ?, 'accepted', ?, ?, 1)`, "batch-task0022", e.admin.TenantID, fixture.plan.Capture.ID, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Two Dataset", "task0022-create")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.datasets.AddCaptures(context.Background(), e.steward, draft.ID, "task0022-add", []string{fixture.plan.Capture.ID}); err != nil {
		t.Fatal(err)
	}
	draft, err = e.datasets.Freeze(context.Background(), e.steward, draft.ID, "task0022-freeze")
	if err != nil {
		t.Fatal(err)
	}
	draft, err = e.datasets.Review(context.Background(), e.reviewer, draft.ID, "task0022-review", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `CREATE TRIGGER task0022_fail_event BEFORE INSERT ON outbox_events BEGIN SELECT RAISE(ABORT, 'release event unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.datasets.Publish(context.Background(), e.steward, draft.ID, "task0022-publish"); err == nil {
		t.Fatal("publish succeeded despite event failure")
	}
	var releases int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM dataset_releases WHERE dataset_id = ?`, draft.ID).Scan(&releases); err != nil {
		t.Fatal(err)
	}
	if releases != 0 {
		t.Fatalf("failed publish left %d visible releases", releases)
	}
}
