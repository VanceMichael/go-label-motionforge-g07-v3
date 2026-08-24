package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/stream"
)

func TestTask0011SegmentBatchFailureRollsBackAllItems(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE capture_sessions SET status = 'recording', version = version + 1 WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE stream_manifests SET status = 'open', digest = '', version = version + 1 WHERE id = ?`, fixture.pose.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER task0011_fail_second BEFORE INSERT ON stream_segments
		WHEN NEW.sequence = 3 BEGIN SELECT RAISE(ABORT, 'object storage rejected segment'); END`); err != nil {
		t.Fatal(err)
	}
	inputs := []stream.SegmentInput{
		{Sequence: 2, StartNanos: 3_000_000, EndNanos: 4_000_000, ObjectURI: "s3://task0011/2", Checksum: domain.Fingerprint("task0011-2"), IdempotencyKey: "task0011-2"},
		{Sequence: 3, StartNanos: 4_000_000, EndNanos: 5_000_000, ObjectURI: "s3://task0011/3", Checksum: domain.Fingerprint("task0011-3"), IdempotencyKey: "task0011-3"},
	}
	if _, err := e.streams.Append(context.Background(), e.operator, fixture.pose.ID, "task0011-append", inputs); err == nil {
		t.Fatal("segment batch succeeded despite second insert failure")
	}
	var count int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM stream_segments WHERE manifest_id = ?`, fixture.pose.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("partial segment batch persisted %d segments; want original 2", count)
	}
}
