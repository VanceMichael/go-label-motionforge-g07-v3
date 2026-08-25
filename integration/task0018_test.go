package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0018AnnotationSubmitRequiresCompleteOwnedBatch(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	batch, items, err := e.annotations.Create(context.Background(), e.steward, fixture.plan.Capture.ID, "task0018-create")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("fixture produced no annotation items")
	}
	claim, err := e.annotations.Claim(context.Background(), e.reviewer, batch.ID, "task0018-claim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.annotations.Submit(context.Background(), e.reviewer, batch.ID, claim.Batch.LeaseToken, "task0018-submit"); err == nil {
		t.Fatal("incomplete annotation batch was submitted")
	}
	current, _, err := e.annotations.Get(context.Background(), e.reviewer, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AnnotationClaimed || current.Owner != e.reviewer.UserID {
		t.Fatalf("incomplete submission polluted claim state: %+v", current)
	}
}
