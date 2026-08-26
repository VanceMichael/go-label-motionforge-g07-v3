package integration_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/annotation"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/clock"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/dataset"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/facility"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/outbox"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/recovery"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/stream"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/training"
)

type testEnvironment struct {
	database     *storage.Database
	databasePath string
	clock        *clock.Manual
	auth         *auth.Service
	facilities   *facility.Service
	captures     *capture.Service
	streams      *stream.Service
	annotations  *annotation.Service
	datasets     *dataset.Service
	training     *training.Service
	outbox       *outbox.Service
	recovery     *recovery.Service
	admin        auth.Principal
	operator     auth.Principal
	reviewer     auth.Principal
	steward      auth.Principal
	worker       auth.Principal
}

func newEnvironment(t *testing.T) *testEnvironment {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "integration.db")
	database, err := storage.Open(context.Background(), storage.Options{Path: databasePath, MaxOpenConns: 8, BusyTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close integration database: %v", err)
		}
	})
	manualClock := clock.NewManual(time.Date(2026, time.August, 24, 2, 0, 0, 0, time.UTC))
	environment := &testEnvironment{
		database:     database,
		databasePath: databasePath,
		clock:        manualClock,
		auth:         auth.NewService(database, manualClock, 8*time.Hour),
		facilities:   facility.NewService(database, manualClock, 2*time.Minute),
		captures:     capture.NewService(database, manualClock, 2*time.Minute),
		streams:      stream.NewService(database, manualClock),
		annotations:  annotation.NewService(database, manualClock, 2*time.Minute),
		datasets:     dataset.NewService(database, manualClock),
		training:     training.NewService(database, manualClock, 2*time.Minute, time.Second, 3),
		outbox:       outbox.NewService(database, manualClock, 2*time.Minute, time.Second),
		recovery:     recovery.NewService(database, manualClock),
	}
	tenant, adminUser, err := environment.auth.Bootstrap(context.Background(), auth.BootstrapInput{
		TenantName:  "Motion Lab",
		Email:       "admin@motion.test",
		DisplayName: "Admin",
		Password:    "test-password-admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	environment.admin = auth.Principal{TenantID: tenant.ID, UserID: adminUser.ID, Role: adminUser.Role, SessionID: "fixture-admin"}
	environment.operator = environment.createPrincipal(t, "operator@motion.test", "Capture Operator", domain.RoleOperator)
	environment.reviewer = environment.createPrincipal(t, "reviewer@motion.test", "Data Reviewer", domain.RoleReviewer)
	environment.steward = environment.createPrincipal(t, "steward@motion.test", "Data Steward", domain.RoleDataSteward)
	environment.worker = environment.createPrincipal(t, "worker@motion.test", "Training Worker", domain.RoleWorker)
	return environment
}

func (e *testEnvironment) createPrincipal(t *testing.T, email, name string, role domain.Role) auth.Principal {
	t.Helper()
	user, err := e.auth.CreateUser(context.Background(), e.admin, email, name, "test-password-user", role, "fixture-user-"+string(role))
	if err != nil {
		t.Fatal(err)
	}
	return auth.Principal{TenantID: e.admin.TenantID, UserID: user.ID, Role: user.Role, SessionID: "fixture-" + string(role)}
}

type captureFixture struct {
	facility domain.Facility
	rig      domain.CaptureRig
	scenario domain.Scenario
	plan     capture.PlanResult
	pose     domain.StreamManifest
	force    domain.StreamManifest
}

func (e *testEnvironment) validatedCapture(t *testing.T) captureFixture {
	t.Helper()
	ctx := context.Background()
	facilityValue, err := e.facilities.CreateFacility(ctx, e.admin, "Precision Lab", "Asia/Shanghai", "fixture-facility")
	if err != nil {
		t.Fatal(err)
	}
	capabilities := domain.CapabilityPose | domain.CapabilityForce | domain.CapabilityTrajectory
	rig, err := e.facilities.CreateRig(ctx, e.admin, facilityValue.ID, "Rig Alpha", capabilities, "fixture-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := e.facilities.CreateScenario(ctx, e.admin, "Shelf grasp", "retail", domain.CapabilityPose|domain.CapabilityForce, "fixture-scenario")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.captures.Plan(ctx, e.operator, capture.PlanInput{FacilityID: facilityValue.ID, ScenarioID: scenario.ID, RigID: rig.ID, ConsentRef: "consent-fixture", IdempotencyKey: "fixture-capture", RequestID: "fixture-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.MarkReady(ctx, e.operator, plan.Capture.ID, "fixture-ready"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.Start(ctx, e.operator, plan.Capture.ID, "fixture-start"); err != nil {
		t.Fatal(err)
	}
	pose, err := e.streams.OpenManifest(ctx, e.operator, plan.Capture.ID, domain.StreamPose, "fixture-pose")
	if err != nil {
		t.Fatal(err)
	}
	force, err := e.streams.OpenManifest(ctx, e.operator, plan.Capture.ID, domain.StreamForce, "fixture-force")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range []domain.StreamManifest{pose, force} {
		inputs := []stream.SegmentInput{
			{Sequence: 0, StartNanos: 1_000_000, EndNanos: 2_000_000, ObjectURI: "s3://fixture/" + manifest.ID + "/0", Checksum: checksum("first-" + manifest.ID), IdempotencyKey: "segment-0"},
			{Sequence: 1, StartNanos: 2_000_000, EndNanos: 3_000_000, ObjectURI: "s3://fixture/" + manifest.ID + "/1", Checksum: checksum("second-" + manifest.ID), IdempotencyKey: "segment-1"},
		}
		if _, err := e.streams.Append(ctx, e.operator, manifest.ID, "fixture-append", inputs); err != nil {
			t.Fatal(err)
		}
		if _, err := e.streams.Seal(ctx, e.operator, manifest.ID, "fixture-seal"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.streams.AlignCapture(ctx, e.operator, plan.Capture.ID, "fixture-align", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.Submit(ctx, e.operator, plan.Capture.ID, "fixture-submit"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.captures.Validate(ctx, e.reviewer, plan.Capture.ID, "fixture-validate"); err != nil {
		t.Fatal(err)
	}
	return captureFixture{facility: facilityValue, rig: rig, scenario: scenario, plan: plan, pose: pose, force: force}
}

func checksum(value string) string {
	return domain.Fingerprint(value)
}

// acceptedCapture returns a validated capture with an accepted annotation
// batch so the capture satisfies dataset eligibility preconditions.
func (e *testEnvironment) acceptedCapture(t *testing.T) captureFixture {
	t.Helper()
	ctx := context.Background()
	fixture := e.validatedCapture(t)
	batch, items, err := e.annotations.Create(ctx, e.steward, fixture.plan.Capture.ID, "fixture-batch")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := e.annotations.Claim(ctx, e.reviewer, batch.ID, "fixture-batch-claim")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if _, err := e.annotations.Annotate(ctx, e.reviewer, batch.ID, claim.Batch.LeaseToken, item.ID, "grasp", `{"quality":"accepted"}`, "fixture-item", item.Version); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.annotations.Submit(ctx, e.reviewer, batch.ID, claim.Batch.LeaseToken, "fixture-batch-submit"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.annotations.Review(ctx, e.steward, batch.ID, "fixture-batch-review", true, ""); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func assertErrorKind(t *testing.T, err, kind error) {
	t.Helper()
	if !errors.Is(err, kind) {
		t.Fatalf("error = %v, want kind %v", err, kind)
	}
}
