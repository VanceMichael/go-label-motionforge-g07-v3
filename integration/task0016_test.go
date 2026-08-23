package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0016ReworkClaimRejectsActiveOwnerOverwrite(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	batch, _, err := e.annotations.Create(context.Background(), e.steward, fixture.plan.Capture.ID, "task0016-create")
	if err != nil {
		t.Fatal(err)
	}
	claimedFixture, err := e.annotations.Claim(context.Background(), e.reviewer, batch.ID, "task0016-prepare-claim")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range claimedFixture.Items {
		if _, err := e.annotations.Annotate(context.Background(), e.reviewer, batch.ID, claimedFixture.Batch.LeaseToken, item.ID, "grasp", `{"quality":"accepted"}`, "task0016-prepare-item", item.Version); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.annotations.Submit(context.Background(), e.reviewer, batch.ID, claimedFixture.Batch.LeaseToken, "task0016-prepare-submit"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.annotations.Review(context.Background(), e.steward, batch.ID, "task0016-prepare-rework", false, "needs another pass"); err != nil {
		t.Fatal(err)
	}
	second := e.createPrincipal(t, "reviewer-two@motion.test", "Second Reviewer", domain.RoleReviewer)
	if _, err := e.annotations.Claim(context.Background(), e.reviewer, batch.ID, "task0016-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.annotations.Claim(context.Background(), second, batch.ID, "task0016-b"); err == nil {
		t.Fatal("active rework claim was overwritten by a second reviewer")
	}
	claimed, _, err := e.annotations.Get(context.Background(), e.reviewer, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != domain.AnnotationClaimed || claimed.Owner == "" {
		t.Fatalf("invalid final claim state: %+v", claimed)
	}
}
