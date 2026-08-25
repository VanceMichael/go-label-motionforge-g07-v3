package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0017StaleAnnotationRenewalCannotOverwriteNewOwner(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	batch, _, err := e.annotations.Create(context.Background(), e.steward, fixture.plan.Capture.ID, "task0017-create")
	if err != nil {
		t.Fatal(err)
	}
	old, err := e.annotations.Claim(context.Background(), e.reviewer, batch.ID, "task0017-claim-a")
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(3 * time.Minute)
	if _, err := e.recovery.RecoverExpired(context.Background(), e.admin.TenantID, e.admin.UserID, "task0017-recover"); err != nil {
		t.Fatal(err)
	}
	second := e.createPrincipal(t, "reviewer-seventeen@motion.test", "Replacement Reviewer", domain.RoleReviewer)
	newClaim, err := e.annotations.Claim(context.Background(), second, batch.ID, "task0017-claim-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.annotations.Renew(context.Background(), e.reviewer, batch.ID, old.Batch.LeaseToken, "task0017-stale-renew", old.Batch.Version); err == nil {
		t.Fatal("stale annotation renewal succeeded")
	}
	current, _, err := e.annotations.Get(context.Background(), second, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Owner != newClaim.Batch.Owner || current.LeaseToken != newClaim.Batch.LeaseToken {
		t.Fatalf("stale renewal replaced current claim: current=%+v new=%+v", current, newClaim.Batch)
	}
}
