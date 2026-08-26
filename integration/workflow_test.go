package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/annotation"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

func TestEndToEndCaptureAnnotationDatasetTraining(t *testing.T) {
	environment := newEnvironment(t)
	fixture := environment.validatedCapture(t)
	ctx := context.Background()

	batch, items, err := environment.annotations.Create(ctx, environment.steward, fixture.plan.Capture.ID, "workflow-batch")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("annotation item count = %d, want 4", len(items))
	}
	claim, err := environment.annotations.Claim(ctx, environment.reviewer, batch.ID, "workflow-claim")
	if err != nil {
		t.Fatal(err)
	}
	if claim.Batch.Owner != environment.reviewer.UserID || claim.Batch.LeaseToken == "" {
		t.Fatalf("claim does not identify current reviewer: %+v", claim.Batch)
	}
	for index, item := range claim.Items {
		updated, err := environment.annotations.Annotate(ctx, environment.reviewer, batch.ID, claim.Batch.LeaseToken, item.ID, "grasp", `{"quality":"accepted"}`, "workflow-item", item.Version)
		if err != nil {
			t.Fatalf("annotate item %d: %v", index, err)
		}
		if !updated.Complete || updated.Label != "grasp" {
			t.Fatalf("annotation item %d was not completed: %+v", index, updated)
		}
	}
	currentBatch, _, err := environment.annotations.Get(ctx, environment.reviewer, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := environment.annotations.Submit(ctx, environment.reviewer, batch.ID, claim.Batch.LeaseToken, "workflow-submit")
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Status != domain.AnnotationSubmitted || submitted.Owner != "" || submitted.LeaseToken != "" {
		t.Fatalf("unexpected submitted batch: %+v", submitted)
	}
	if submitted.Version != currentBatch.Version+1 {
		t.Fatalf("submit version = %d, want %d", submitted.Version, currentBatch.Version+1)
	}
	accepted, err := environment.annotations.Review(ctx, environment.steward, batch.ID, "workflow-review", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != domain.AnnotationAccepted {
		t.Fatalf("review status = %s", accepted.Status)
	}

	draft, err := environment.datasets.Create(ctx, environment.steward, "Retail grasp set", "workflow-dataset")
	if err != nil {
		t.Fatal(err)
	}
	draft, datasetItems, err := environment.datasets.AddCaptures(ctx, environment.steward, draft.ID, "workflow-dataset-items", []string{fixture.plan.Capture.ID})
	if err != nil {
		t.Fatal(err)
	}
	if draft.ItemCount != 1 || len(datasetItems) != 1 {
		t.Fatalf("dataset membership = %d/%d, want 1/1", draft.ItemCount, len(datasetItems))
	}
	frozen, err := environment.datasets.Freeze(ctx, environment.steward, draft.ID, "workflow-freeze")
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Status != domain.DatasetStatusFrozen || len(frozen.Digest) != 64 || frozen.FrozenAt == nil {
		t.Fatalf("unexpected frozen dataset: %+v", frozen)
	}
	approved, err := environment.datasets.Review(ctx, environment.reviewer, draft.ID, "workflow-quality", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.DatasetStatusApproved {
		t.Fatalf("quality status = %s", approved.Status)
	}
	release, err := environment.datasets.Publish(ctx, environment.steward, draft.ID, "workflow-publish")
	if err != nil {
		t.Fatal(err)
	}
	if release.Status != domain.DatasetStatusPublished || release.Digest != frozen.Digest {
		t.Fatalf("published release does not preserve frozen digest: %+v", release)
	}

	job, err := environment.training.Enqueue(ctx, environment.steward, release.ID, "workflow-enqueue")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != domain.JobQueued || job.MaxAttempts != 3 {
		t.Fatalf("unexpected queued job: %+v", job)
	}
	claimed, err := environment.training.Claim(ctx, environment.worker)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Job.ID != job.ID || claimed.Job.Status != domain.JobRunning || claimed.Attempt.Attempt != 1 {
		t.Fatalf("unexpected claimed job: %+v", claimed)
	}
	checkpointed, err := environment.training.Checkpoint(ctx, environment.worker, job.ID, claimed.Job.LeaseToken, "artifact-uploaded", claimed.Job.Version)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := environment.training.Complete(ctx, environment.worker, job.ID, checkpointed.LeaseToken, "s3://models/model-1", "workflow-complete", checkpointed.Version)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domain.JobSucceeded || completed.OutputURI == "" || completed.Owner != "" {
		t.Fatalf("unexpected completed job: %+v", completed)
	}
	var attemptOutcome string
	if err := environment.database.SQL().QueryRow(`SELECT outcome FROM job_attempts WHERE job_id = ? AND attempt = 1`, job.ID).Scan(&attemptOutcome); err != nil {
		t.Fatal(err)
	}
	if attemptOutcome != "succeeded" {
		t.Fatalf("attempt outcome = %q", attemptOutcome)
	}
}

func TestCapturePlanReplayIsScopedAndExact(t *testing.T) {
	environment := newEnvironment(t)
	ctx := context.Background()
	facilityValue, err := environment.facilities.CreateFacility(ctx, environment.admin, "Replay Lab", "UTC", "replay-facility")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := environment.facilities.CreateRig(ctx, environment.admin, facilityValue.ID, "Replay Rig", domain.CapabilityPose, "replay-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := environment.facilities.CreateScenario(ctx, environment.admin, "Replay Pose", "office", domain.CapabilityPose, "replay-scenario")
	if err != nil {
		t.Fatal(err)
	}
	input := capture.PlanInput{FacilityID: facilityValue.ID, ScenarioID: scenario.ID, RigID: rig.ID, ConsentRef: "consent-replay", IdempotencyKey: "same-key", RequestID: "replay-first"}
	first, err := environment.captures.Plan(ctx, environment.operator, input)
	if err != nil {
		t.Fatal(err)
	}
	input.RequestID = "replay-second"
	second, err := environment.captures.Plan(ctx, environment.operator, input)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replay || first.Capture.ID != second.Capture.ID || first.Lease.ID != second.Lease.ID || first.Lease.Token != second.Lease.Token {
		t.Fatalf("replay changed durable result: first=%+v second=%+v", first, second)
	}
	var captureCount, leaseCount int
	if err := environment.database.SQL().QueryRow(`SELECT COUNT(*) FROM capture_sessions`).Scan(&captureCount); err != nil {
		t.Fatal(err)
	}
	if err := environment.database.SQL().QueryRow(`SELECT COUNT(*) FROM rig_leases`).Scan(&leaseCount); err != nil {
		t.Fatal(err)
	}
	if captureCount != 1 || leaseCount != 1 {
		t.Fatalf("replay duplicated side effects: captures=%d leases=%d", captureCount, leaseCount)
	}
	input.ConsentRef = "changed-consent"
	if _, err := environment.captures.Plan(ctx, environment.operator, input); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("changed replay error = %v, want idempotency conflict", err)
	}
}

func TestAuditFailureRollsBackCaptureTransition(t *testing.T) {
	environment := newEnvironment(t)
	ctx := context.Background()
	facilityValue, err := environment.facilities.CreateFacility(ctx, environment.admin, "Rollback Lab", "UTC", "rollback-facility")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := environment.facilities.CreateRig(ctx, environment.admin, facilityValue.ID, "Rollback Rig", domain.CapabilityPose, "rollback-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := environment.facilities.CreateScenario(ctx, environment.admin, "Rollback Pose", "home", domain.CapabilityPose, "rollback-scenario")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := environment.captures.Plan(ctx, environment.operator, capture.PlanInput{FacilityID: facilityValue.ID, ScenarioID: scenario.ID, RigID: rig.ID, ConsentRef: "consent", IdempotencyKey: "rollback-plan", RequestID: "rollback-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.database.SQL().Exec(`
		CREATE TRIGGER fail_ready_audit BEFORE INSERT ON audit_events
		WHEN NEW.action = 'capture.ready'
		BEGIN SELECT RAISE(ABORT, 'forced audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.captures.MarkReady(ctx, environment.operator, plan.Capture.ID, "rollback-ready"); err == nil {
		t.Fatal("transition unexpectedly succeeded while audit insert failed")
	}
	current, err := environment.captures.Get(ctx, environment.operator, plan.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.CapturePlanned || current.Version != plan.Capture.Version {
		t.Fatalf("failed transition leaked state: %+v", current)
	}
	var readyEvents int
	if err := environment.database.SQL().QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE topic = 'capture.ready'`).Scan(&readyEvents); err != nil {
		t.Fatal(err)
	}
	if readyEvents != 0 {
		t.Fatalf("failed transition leaked %d outbox events", readyEvents)
	}
}

func TestCanceledContextCannotStartCapture(t *testing.T) {
	environment := newEnvironment(t)
	ctx := context.Background()
	facilityValue, err := environment.facilities.CreateFacility(ctx, environment.admin, "Cancel Lab", "UTC", "cancel-facility")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := environment.facilities.CreateRig(ctx, environment.admin, facilityValue.ID, "Cancel Rig", domain.CapabilityPose, "cancel-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := environment.facilities.CreateScenario(ctx, environment.admin, "Cancel Pose", "factory", domain.CapabilityPose, "cancel-scenario")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := environment.captures.Plan(ctx, environment.operator, capture.PlanInput{FacilityID: facilityValue.ID, ScenarioID: scenario.ID, RigID: rig.ID, ConsentRef: "consent", IdempotencyKey: "cancel-plan", RequestID: "cancel-plan"})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := environment.captures.MarkReady(ctx, environment.operator, plan.Capture.ID, "cancel-ready")
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := environment.captures.Start(canceled, environment.operator, plan.Capture.ID, "cancel-start"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v, want context canceled", err)
	}
	current, err := environment.captures.Get(ctx, environment.operator, plan.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.CaptureReady || current.Version != ready.Version || current.StartedAt != nil {
		t.Fatalf("canceled start changed capture: %+v", current)
	}
}

func TestConcurrentAnnotationClaimHasOneOwner(t *testing.T) {
	environment := newEnvironment(t)
	fixture := environment.validatedCapture(t)
	batch, _, err := environment.annotations.Create(context.Background(), environment.steward, fixture.plan.Capture.ID, "claim-batch")
	if err != nil {
		t.Fatal(err)
	}
	secondReviewer := environment.createPrincipal(t, "reviewer2@motion.test", "Second Reviewer", domain.RoleReviewer)
	start := make(chan struct{})
	results := make(chan annotation.ClaimResult, 2)
	errorsCh := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	claim := func(principal auth.Principal) {
		ready.Done()
		<-start
		result, err := environment.annotations.Claim(context.Background(), principal, batch.ID, "concurrent-claim")
		if err != nil {
			errorsCh <- err
			return
		}
		results <- result
	}
	go claim(environment.reviewer)
	go claim(secondReviewer)
	ready.Wait()
	close(start)
	var successCount, conflictCount int
	for i := 0; i < 2; i++ {
		select {
		case result := <-results:
			successCount++
			if result.Batch.Owner == "" || result.Batch.LeaseToken == "" {
				t.Errorf("successful claim lacks ownership: %+v", result.Batch)
			}
		case err := <-errorsCh:
			if errors.Is(err, domain.ErrConflict) {
				conflictCount++
			} else {
				t.Errorf("unexpected claim error: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent claim did not finish")
		}
	}
	if successCount != 1 || conflictCount != 1 {
		t.Fatalf("claim outcomes success=%d conflict=%d, want 1/1", successCount, conflictCount)
	}
}

func TestRecoveryIsAtomicAcrossExpiredResources(t *testing.T) {
	environment := newEnvironment(t)
	fixture := environment.validatedCapture(t)
	batch, _, err := environment.annotations.Create(context.Background(), environment.steward, fixture.plan.Capture.ID, "recovery-batch")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := environment.annotations.Claim(context.Background(), environment.reviewer, batch.ID, "recovery-claim")
	if err != nil {
		t.Fatal(err)
	}
	environment.clock.Advance(3 * time.Minute)
	if _, err := environment.database.SQL().Exec(`
		CREATE TRIGGER fail_outbox_recovery BEFORE UPDATE ON outbox_events
		WHEN OLD.status = 'delivering'
		BEGIN SELECT RAISE(ABORT, 'forced recovery failure'); END`); err != nil {
		t.Fatal(err)
	}
	now := storage.FormatTime(environment.clock.Now().Add(-time.Minute))
	if _, err := environment.database.SQL().Exec(`
		UPDATE outbox_events SET status = 'delivering', owner = 'old-worker', lease_token = 'old-token', lease_expires_at = ?
		WHERE id = (SELECT id FROM outbox_events LIMIT 1)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.recovery.RecoverExpired(context.Background(), environment.admin.TenantID, environment.admin.UserID, "recovery-run"); err == nil {
		t.Fatal("recovery unexpectedly succeeded through forced outbox failure")
	}
	current, _, err := environment.annotations.Get(context.Background(), environment.reviewer, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AnnotationClaimed || current.Owner != claim.Batch.Owner {
		t.Fatalf("failed recovery partially reset annotation claim: %+v", current)
	}
	if _, err := environment.database.SQL().Exec(`DROP TRIGGER fail_outbox_recovery`); err != nil {
		t.Fatal(err)
	}
	result, err := environment.recovery.RecoverExpired(context.Background(), environment.admin.TenantID, environment.admin.UserID, "recovery-retry")
	if err != nil {
		t.Fatal(err)
	}
	if result.AnnotationBatches != 1 || result.OutboxEvents == 0 {
		t.Fatalf("unexpected recovery result: %+v", result)
	}
	current, _, err = environment.annotations.Get(context.Background(), environment.reviewer, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AnnotationOpen || current.Owner != "" || current.LeaseToken != "" {
		t.Fatalf("successful recovery did not reopen batch: %+v", current)
	}
}

func TestRestartRetainsWorkflowState(t *testing.T) {
	environment := newEnvironment(t)
	fixture := environment.validatedCapture(t)
	if err := environment.database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.Open(context.Background(), storage.Options{Path: environment.databasePath, MaxOpenConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var status domain.CaptureStatus
	var manifestCount int
	if err := reopened.SQL().QueryRow(`SELECT status FROM capture_sessions WHERE id = ?`, fixture.plan.Capture.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := reopened.SQL().QueryRow(`SELECT COUNT(*) FROM stream_manifests WHERE capture_id = ? AND status = 'aligned'`, fixture.plan.Capture.ID).Scan(&manifestCount); err != nil {
		t.Fatal(err)
	}
	if status != domain.CaptureValidated || manifestCount != 2 {
		t.Fatalf("restart state status=%s aligned=%d, want validated/2", status, manifestCount)
	}
}

func TestRevokeBlockedWhileTrainingJobActive(t *testing.T) {
	environment := newEnvironment(t)
	fixture := environment.acceptedCapture(t)
	ctx := context.Background()

	draft, err := environment.datasets.Create(ctx, environment.steward, "Revoke guard set", "guard-dataset")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := environment.datasets.AddCaptures(ctx, environment.steward, draft.ID, "guard-items", []string{fixture.plan.Capture.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.datasets.Freeze(ctx, environment.steward, draft.ID, "guard-freeze"); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.datasets.Review(ctx, environment.reviewer, draft.ID, "guard-quality", true, ""); err != nil {
		t.Fatal(err)
	}
	release, err := environment.datasets.Publish(ctx, environment.steward, draft.ID, "guard-publish")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.training.Enqueue(ctx, environment.steward, release.ID, "guard-enqueue"); err != nil {
		t.Fatal(err)
	}
	claimed, err := environment.training.Claim(ctx, environment.worker)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Job.Status != domain.JobRunning {
		t.Fatalf("claim status = %s, want running", claimed.Job.Status)
	}
	if _, err := environment.datasets.Revoke(ctx, environment.steward, release.ID, "guard-revoke", "retired"); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("revoke active release error = %v, want precondition", err)
	}
	persisted, err := environment.datasets.GetRelease(ctx, environment.steward, release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != domain.DatasetStatusPublished || persisted.RevokedAt != nil {
		t.Fatalf("revoked-attempted release changed: %+v", persisted)
	}
	checkpointed, err := environment.training.Checkpoint(ctx, environment.worker, claimed.Job.ID, claimed.Job.LeaseToken, "guard-checkpoint", claimed.Job.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.training.Complete(ctx, environment.worker, claimed.Job.ID, checkpointed.LeaseToken, "s3://models/guard", "guard-complete", checkpointed.Version); err != nil {
		t.Fatal(err)
	}
	revoked, err := environment.datasets.Revoke(ctx, environment.steward, release.ID, "guard-revoke-after", "retired")
	if err != nil {
		t.Fatalf("revoke after completion error = %v", err)
	}
	if revoked.Status != domain.DatasetStatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("unexpected revoked release: %+v", revoked)
	}
}

func TestClaimRefusesRevokedRelease(t *testing.T) {
	environment := newEnvironment(t)
	fixture := environment.acceptedCapture(t)
	ctx := context.Background()

	draft, err := environment.datasets.Create(ctx, environment.steward, "Claim guard set", "claim-guard-dataset")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := environment.datasets.AddCaptures(ctx, environment.steward, draft.ID, "claim-guard-items", []string{fixture.plan.Capture.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.datasets.Freeze(ctx, environment.steward, draft.ID, "claim-guard-freeze"); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.datasets.Review(ctx, environment.reviewer, draft.ID, "claim-guard-quality", true, ""); err != nil {
		t.Fatal(err)
	}
	release, err := environment.datasets.Publish(ctx, environment.steward, draft.ID, "claim-guard-publish")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.datasets.Revoke(ctx, environment.steward, release.ID, "claim-guard-revoke", "retired"); err != nil {
		t.Fatalf("revoke before claim error = %v", err)
	}
	// Simulate a stale queued job that references the now-revoked release.
	now := storage.FormatTime(environment.clock.Now())
	if _, err := environment.database.SQL().Exec(`
		INSERT INTO training_jobs(
			id, tenant_id, release_id, status, owner, lease_token,
			lease_expires_at, attempt_count, max_attempts, checkpoint,
			output_uri, last_error, next_attempt_at, created_at, updated_at, version
		) VALUES('job_stale_revoked', ?, ?, 'queued', '', '', NULL, 0, 3, '', '', '', ?, ?, ?, 1)`,
		environment.admin.TenantID, release.ID, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.training.Claim(ctx, environment.worker); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("claim revoked release error = %v, want precondition", err)
	}
	persisted, err := environment.training.Get(ctx, environment.worker, "job_stale_revoked")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != domain.JobQueued || persisted.Owner != "" {
		t.Fatalf("claim against revoked release mutated job: %+v", persisted)
	}
}

var _ *sql.DB
