package integration_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0008CancelFailurePreservesCaptureAndLease(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		UPDATE capture_sessions SET status = 'planned', submitted_at = NULL, validated_at = NULL, version = version + 1 WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER task0008_fail_audit BEFORE INSERT ON audit_events
		BEGIN SELECT RAISE(ABORT, 'audit sink unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.Cancel(context.Background(), e.operator, fixture.plan.Capture.ID, "task0008-cancel"); err == nil {
		t.Fatal("cancel succeeded despite rejected audit event")
	}
	current, err := e.captures.Get(context.Background(), e.operator, fixture.plan.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.CapturePlanned {
		t.Fatalf("failed cancel changed capture status to %s", current.Status)
	}
	var released sql.NullString
	if err := e.database.SQL().QueryRowContext(context.Background(), `
		SELECT released_at FROM rig_leases WHERE id = ?`, fixture.plan.Lease.ID).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if released.Valid {
		t.Fatalf("failed cancel released the rig at %s", released.String)
	}
}
