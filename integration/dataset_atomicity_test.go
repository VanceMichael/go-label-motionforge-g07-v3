package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

// TestAddCapturesAtomicOnEligibilityFailure guards the dataset membership
// atomicity contract: when any capture in a batch fails eligibility, no member
// may be persisted to the draft. This regression previously left the first
// capture written because it was inserted outside the transaction.
func TestAddCapturesAtomicOnEligibilityFailure(t *testing.T) {
	environment := newEnvironment(t)
	ctx := context.Background()

	validFixture := environment.validatedCapture(t)
	batch, _, err := environment.annotations.Create(ctx, environment.steward, validFixture.plan.Capture.ID, "atomic-batch")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := environment.annotations.Claim(ctx, environment.reviewer, batch.ID, "atomic-claim")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range claim.Items {
		if _, err := environment.annotations.Annotate(ctx, environment.reviewer, batch.ID, claim.Batch.LeaseToken, item.ID, "grasp", `{"quality":"accepted"}`, "atomic-item", item.Version); err != nil {
			t.Fatalf("annotate item %s: %v", item.ID, err)
		}
	}
	if _, err := environment.annotations.Submit(ctx, environment.reviewer, batch.ID, claim.Batch.LeaseToken, "atomic-submit"); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.annotations.Review(ctx, environment.steward, batch.ID, "atomic-review", true, ""); err != nil {
		t.Fatal(err)
	}

	// Second capture is only planned: it is not validated and carries no
	// accepted annotation, so EligibleCapture must reject it.
	facility, err := environment.facilities.CreateFacility(ctx, environment.admin, "Atomic Lab", "UTC", "atomic-facility")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := environment.facilities.CreateRig(ctx, environment.admin, facility.ID, "Atomic Rig", domain.CapabilityPose, "atomic-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := environment.facilities.CreateScenario(ctx, environment.admin, "Atomic Pose", "retail", domain.CapabilityPose, "atomic-scenario")
	if err != nil {
		t.Fatal(err)
	}
	ineligible, err := environment.captures.Plan(ctx, environment.operator, capture.PlanInput{
		FacilityID: facility.ID, ScenarioID: scenario.ID, RigID: rig.ID,
		ConsentRef: "consent-atomic", IdempotencyKey: "atomic-ineligible", RequestID: "atomic-plan",
	})
	if err != nil {
		t.Fatal(err)
	}

	draft, err := environment.datasets.Create(ctx, environment.steward, "Atomic membership set", "atomic-dataset")
	if err != nil {
		t.Fatal(err)
	}

	if _, items, err := environment.datasets.AddCaptures(ctx, environment.steward, draft.ID, "atomic-add", []string{validFixture.plan.Capture.ID, ineligible.Capture.ID}); err == nil {
		t.Fatalf("add captures unexpectedly succeeded with %d items", len(items))
	}

	var memberCount, itemCount int
	if err := environment.database.SQL().QueryRow(`SELECT COUNT(*) FROM dataset_items WHERE dataset_id = ?`, draft.ID).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if err := environment.database.SQL().QueryRow(`SELECT item_count FROM dataset_drafts WHERE id = ?`, draft.ID).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 0 || memberCount != 0 {
		t.Fatalf("partial membership leaked: items=%d item_count=%d", itemCount, memberCount)
	}

	var auditCount, outboxCount int
	if err := environment.database.SQL().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE object_id = ? AND action = 'dataset.members.add'`, draft.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if err := environment.database.SQL().QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = ? AND topic = 'dataset.members.add'`, draft.ID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 0 || outboxCount != 0 {
		t.Fatalf("partial side effects leaked: audit=%d outbox=%d", auditCount, outboxCount)
	}
}
