package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

func TestTask0021FreezeDigestMatchesImmutableMembership(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE capture_sessions SET status = 'planned' WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty One Dataset", "task0021-create")
	if err != nil {
		t.Fatal(err)
	}
	stamp := storage.FormatTime(e.clock.Now())
	if _, err := e.database.SQL().ExecContext(context.Background(), `INSERT INTO dataset_items(id, tenant_id, dataset_id, capture_id, revision, created_at) VALUES(?, ?, ?, ?, ?, ?)`, "item-task0021", e.admin.TenantID, draft.ID, fixture.plan.Capture.ID, draft.Revision, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE dataset_drafts SET item_count = 1 WHERE id = ?`, draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.datasets.Freeze(context.Background(), e.steward, draft.ID, "task0021-freeze"); err == nil {
		t.Fatal("dataset with ineligible capture was frozen")
	}
	current, _, err := e.datasets.Get(context.Background(), e.steward, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.DatasetStatusDraft {
		t.Fatalf("ineligible dataset changed to %s", current.Status)
	}
}
