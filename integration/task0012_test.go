package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/stream"
)

func TestTask0012SegmentReplayIsStable(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE capture_sessions SET status = 'recording', version = version + 1 WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE stream_manifests SET status = 'open', digest = '', version = version + 1 WHERE id = ?`, fixture.pose.ID); err != nil {
		t.Fatal(err)
	}
	input := stream.SegmentInput{Sequence: 2, StartNanos: 3_000_000, EndNanos: 4_000_000, ObjectURI: "s3://task0012/2", Checksum: domain.Fingerprint("task0012"), IdempotencyKey: "task0012-key"}
	first, err := e.streams.Append(context.Background(), e.operator, fixture.pose.ID, "task0012-first", []stream.SegmentInput{input})
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.streams.Append(context.Background(), e.operator, fixture.pose.ID, "task0012-retry", []stream.SegmentInput{input})
	if err != nil {
		t.Fatal(err)
	}
	if second.Replayed != 1 || len(second.Segments) != 1 || second.Segments[0].ID != first.Segments[0].ID {
		t.Fatalf("segment replay was not stable: first=%+v second=%+v", first, second)
	}
}
