package integration_test

import (
	"context"
	"testing"
)

func TestTask0020DatasetMembershipFailureRollsBackBatch(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	draft, err := e.datasets.Create(context.Background(), e.steward, "Task Twenty Dataset", "task0020-create")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.datasets.AddCaptures(context.Background(), e.steward, draft.ID, "task0020-add", []string{fixture.plan.Capture.ID, "missing-capture"}); err == nil {
		t.Fatal("dataset batch accepted an ineligible capture")
	}
	var count int
	if err := e.database.SQL().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM dataset_items WHERE dataset_id = ?`, draft.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed dataset batch left %d membership rows", count)
	}
}
