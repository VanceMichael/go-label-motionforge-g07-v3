package integration_test

import (
	"context"
	"testing"
)

func TestTask0014AlignmentFailureLeavesAllManifestsSealed(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE capture_sessions SET status = 'recording', version = version + 1 WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE stream_manifests SET status = 'sealed', version = version + 1 WHERE capture_id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER task0014_fail_event BEFORE INSERT ON outbox_events
		BEGIN SELECT RAISE(ABORT, 'event bus unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.streams.AlignCapture(context.Background(), e.operator, fixture.plan.Capture.ID, "task0014-align", 1_000_000); err == nil {
		t.Fatal("alignment succeeded despite event publication failure")
	}
	var aligned int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM stream_manifests WHERE capture_id = ? AND status = 'aligned'`, fixture.plan.Capture.ID).Scan(&aligned); err != nil {
		t.Fatal(err)
	}
	if aligned != 0 {
		t.Fatalf("failed alignment committed %d aligned manifests", aligned)
	}
	var sealed int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM stream_manifests WHERE capture_id = ? AND status = 'sealed'`, fixture.plan.Capture.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if sealed != 2 {
		t.Fatalf("expected both manifests to remain sealed, found %d", sealed)
	}
}
