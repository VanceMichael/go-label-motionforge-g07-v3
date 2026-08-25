package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0006CapturePlanReplayDoesNotDuplicateLease(t *testing.T) {
	e := newEnvironment(t)
	facilityValue, err := e.facilities.CreateFacility(context.Background(), e.admin, "Replay Lab", "Asia/Shanghai", "task0006-facility")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := e.facilities.CreateRig(context.Background(), e.admin, facilityValue.ID, "Replay Rig", domain.CapabilityPose, "task0006-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := e.facilities.CreateScenario(context.Background(), e.admin, "Replay scenario", "lab", domain.CapabilityPose, "task0006-scenario")
	if err != nil {
		t.Fatal(err)
	}
	input := capture.PlanInput{FacilityID: facilityValue.ID, ScenarioID: scenario.ID, RigID: rig.ID, ConsentRef: "consent-replay", IdempotencyKey: "task0006-key", RequestID: "task0006-first"}
	first, err := e.captures.Plan(context.Background(), e.operator, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.facilities.ReleaseRig(context.Background(), e.operator, rig.ID, first.Lease.ID, first.Lease.Token, first.Lease.Version, "task0006-release"); err != nil {
		t.Fatal(err)
	}
	input.RequestID = "task0006-retry"
	second, err := e.captures.Plan(context.Background(), e.operator, input)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replay || second.Capture.ID != first.Capture.ID || second.Lease.ID != first.Lease.ID {
		t.Fatalf("idempotent retry allocated a new plan: first=%+v second=%+v", first, second)
	}
}
