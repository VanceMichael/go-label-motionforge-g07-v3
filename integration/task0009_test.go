package integration_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0009SubmitFailureKeepsProcessingLease(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		UPDATE capture_sessions SET status = 'recording', submitted_at = NULL, validated_at = NULL, version = version + 1 WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER task0009_fail_audit BEFORE INSERT ON audit_events
		BEGIN SELECT RAISE(ABORT, 'audit sink unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.Submit(context.Background(), e.operator, fixture.plan.Capture.ID, "task0009-submit"); err == nil {
		t.Fatal("submit succeeded despite rejected audit event")
	}
	current, err := e.captures.Get(context.Background(), e.operator, fixture.plan.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.CaptureRecording {
		t.Fatalf("failed submit changed status to %s", current.Status)
	}
	var released sql.NullString
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT released_at FROM rig_leases WHERE id = ?`, fixture.plan.Lease.ID).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if released.Valid {
		t.Fatalf("failed submit released rig at %s", released.String)
	}
}
