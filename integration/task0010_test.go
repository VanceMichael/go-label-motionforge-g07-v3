package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0010ReopenRequiresAtomicRigLease(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		UPDATE capture_sessions SET status = 'rejected', submitted_at = NULL, validated_at = NULL, version = version + 1 WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.Reopen(context.Background(), e.operator, fixture.plan.Capture.ID, "task0010-reopen"); err == nil {
		t.Fatal("reopen succeeded while its rig was unavailable")
	}
	current, err := e.captures.Get(context.Background(), e.operator, fixture.plan.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.CaptureRejected {
		t.Fatalf("failed reopen advanced capture to %s without a new lease", current.Status)
	}
}
