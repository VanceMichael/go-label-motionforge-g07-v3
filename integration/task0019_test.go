package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0019ReworkFailurePreservesSubmittedBatch(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	batch, items, err := e.annotations.Create(context.Background(), e.steward, fixture.plan.Capture.ID, "task0019-create")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := e.annotations.Claim(context.Background(), e.reviewer, batch.ID, "task0019-claim")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if _, err := e.annotations.Annotate(context.Background(), e.reviewer, batch.ID, claim.Batch.LeaseToken, item.ID, "grasp", `{"quality":"ok"}`, "task0019-item", item.Version); err != nil {
			t.Fatal(err)
		}
	}
	claimed, _, err := e.annotations.Get(context.Background(), e.reviewer, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.annotations.Submit(context.Background(), e.reviewer, batch.ID, claim.Batch.LeaseToken, "task0019-submit"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.annotations.Review(context.Background(), e.steward, batch.ID, "task0019-review", false, "rework labels"); err != nil {
		t.Fatal(err)
	}
	_, reviewedItems, err := e.annotations.Get(context.Background(), e.reviewer, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewedItems) == 0 {
		t.Fatal("reviewed batch lost its items")
	}
	for _, item := range reviewedItems {
		if item.Complete {
			t.Fatalf("rework left item %s complete; claimed version was %d", item.ID, claimed.Version)
		}
	}
	if claimed.Status != domain.AnnotationClaimed {
		t.Fatal("unexpected claim fixture state")
	}
}
