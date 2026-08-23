package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0007CanceledReadyTransitionDoesNotAdvanceCapture(t *testing.T) {
	e := newEnvironment(t)
	facilityValue, err := e.facilities.CreateFacility(context.Background(), e.admin, "Canceled Start Lab", "Asia/Shanghai", "task0007-facility")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := e.facilities.CreateRig(context.Background(), e.admin, facilityValue.ID, "Start Rig", domain.CapabilityPose, "task0007-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := e.facilities.CreateScenario(context.Background(), e.admin, "Start scenario", "lab", domain.CapabilityPose, "task0007-scenario")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.captures.Plan(context.Background(), e.operator, captureInput(facilityValue.ID, scenario.ID, rig.ID, "task0007"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.MarkReady(context.Background(), e.operator, plan.Capture.ID, "task0007-ready"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.captures.MarkReady(ctx, e.operator, plan.Capture.ID, "task0007-ready-canceled"); err == nil {
		t.Fatal("canceled ready transition succeeded")
	}
	current, err := e.captures.Get(context.Background(), e.operator, plan.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.CapturePlanned {
		t.Fatalf("canceled ready transition advanced capture to %s", current.Status)
	}
}

func captureInput(facilityID, scenarioID, rigID, key string) capture.PlanInput {
	return capture.PlanInput{FacilityID: facilityID, ScenarioID: scenarioID, RigID: rigID, ConsentRef: "consent-" + key, IdempotencyKey: key, RequestID: key + "-plan"}
}
