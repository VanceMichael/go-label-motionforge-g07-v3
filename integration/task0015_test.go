package integration_test

import (
	"context"
	"testing"
)

func TestTask0015AnnotationExpansionRollsBackParentAndChildren(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER task0015_fail_item BEFORE INSERT ON annotation_items
		BEGIN SELECT RAISE(ABORT, 'annotation index unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.annotations.Create(context.Background(), e.steward, fixture.plan.Capture.ID, "task0015-create"); err == nil {
		t.Fatal("annotation expansion succeeded despite child persistence failure")
	}
	var batches, items int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM annotation_batches WHERE capture_id = ?`, fixture.plan.Capture.ID).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM annotation_items WHERE tenant_id = ?`, e.admin.TenantID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if batches != 0 || items != 0 {
		t.Fatalf("failed annotation expansion left parent/children: batches=%d items=%d", batches, items)
	}
}
