package capture

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/audit"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/clock"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/facility"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/idempotency"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/outbox"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

const planPath = "/api/v1/captures"

type Service struct {
	db             *storage.Database
	repo           Repository
	facilities     facility.Repository
	audits         audit.Store
	outbox         outbox.Repository
	idempotency    idempotency.Store
	clock          clock.Clock
	leaseTTL       time.Duration
	idempotencyTTL time.Duration
}

type PlanInput struct {
	FacilityID     string
	ScenarioID     string
	RigID          string
	ConsentRef     string
	IdempotencyKey string
	RequestID      string
}

type PlanResult struct {
	Capture domain.CaptureSession `json:"capture"`
	Lease   domain.RigLease       `json:"lease"`
	Replay  bool                  `json:"replay"`
}

func NewService(db *storage.Database, c clock.Clock, leaseTTL time.Duration) *Service {
	return &Service{
		db: db, repo: Repository{}, facilities: facility.Repository{}, audits: audit.Store{},
		outbox: outbox.Repository{}, idempotency: idempotency.Store{}, clock: c,
		leaseTTL: leaseTTL, idempotencyTTL: 24 * time.Hour,
	}
}

func (s *Service) Plan(ctx context.Context, principal auth.Principal, input PlanInput) (PlanResult, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return PlanResult{}, err
	}
	if err := validatePlan(input); err != nil {
		return PlanResult{}, err
	}
	fingerprint := domain.Fingerprint(input.FacilityID, input.ScenarioID, input.RigID, input.ConsentRef)
	now := s.clock.Now()
	var result PlanResult
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		record, found, err := s.idempotency.Lookup(ctx, tx, principal.TenantID, http.MethodPost, planPath, input.IdempotencyKey, now)
		if err != nil {
			return err
		}
		if found {
			if err := idempotency.EnsureFingerprint(record, fingerprint); err != nil {
				return err
			}
			if err := json.Unmarshal(record.Response, &result); err != nil {
				return fmt.Errorf("decode plan replay: %w", err)
			}
			// The durable response is the source of truth for the lifetime of
			// the idempotency record, regardless of the original rig lease's
			// lifecycle. A released or expired lease never justifies silently
			// reallocating a new capture or device lease on replay; callers that
			// need a fresh lease use Reopen or ReserveRig against this capture.
			result.Replay = true
			return nil
		}
		facilityValue, err := s.facilities.FindFacility(ctx, tx, principal.TenantID, input.FacilityID)
		if err != nil {
			return err
		}
		rig, err := s.facilities.FindRig(ctx, tx, principal.TenantID, input.RigID)
		if err != nil {
			return err
		}
		scenario, err := s.facilities.FindScenario(ctx, tx, principal.TenantID, input.ScenarioID)
		if err != nil {
			return err
		}
		if !facilityValue.Active || !rig.Active || !scenario.Active {
			return domain.Precondition("capture.plan", "capture_session", "", "facility, rig, and scenario must be active")
		}
		if rig.FacilityID != facilityValue.ID {
			return domain.Precondition("capture.plan", "capture_rig", rig.ID, "rig belongs to another facility")
		}
		if !rig.Supports(scenario.RequiredCapabilities) {
			return domain.Precondition("capture.plan", "capture_rig", rig.ID, "rig does not satisfy scenario capabilities")
		}
		captureID, err := domain.NewID("capture")
		if err != nil {
			return err
		}
		leaseID, err := domain.NewID("lease")
		if err != nil {
			return err
		}
		token, err := randomLeaseToken()
		if err != nil {
			return err
		}
		captureValue := domain.CaptureSession{
			ID: captureID, TenantID: principal.TenantID, FacilityID: facilityValue.ID,
			ScenarioID: scenario.ID, RigID: rig.ID, OperatorID: principal.UserID,
			Status: domain.CapturePlanned, Revision: 1, ConsentRef: input.ConsentRef,
			CreatedAt: now, UpdatedAt: now, Version: 1,
		}
		lease := domain.RigLease{
			ID: leaseID, TenantID: principal.TenantID, RigID: rig.ID,
			CaptureID: captureID, Owner: principal.UserID, Token: token,
			ExpiresAt: now.Add(s.leaseTTL), CreatedAt: now, UpdatedAt: now, Version: 1,
		}
		if err := s.repo.Insert(ctx, tx, captureValue); err != nil {
			return err
		}
		if err := s.facilities.InsertLease(ctx, tx, lease); err != nil {
			return domain.Conflict("capture.plan", "capture_rig", rig.ID, "rig already has a live lease")
		}
		if err := s.appendEffects(ctx, tx, principal, input.RequestID, "capture.plan", captureValue.ID, "planned", now); err != nil {
			return err
		}
		result = PlanResult{Capture: captureValue, Lease: lease}
		response, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("encode plan response: %w", err)
		}
		recordID, err := domain.NewID("idem")
		if err != nil {
			return err
		}
		return s.idempotency.Save(ctx, tx, domain.IdempotencyRecord{
			ID: recordID, TenantID: principal.TenantID, Method: http.MethodPost,
			Path: planPath, Key: input.IdempotencyKey, Fingerprint: fingerprint,
			StatusCode: http.StatusCreated, Response: response, CreatedAt: now,
			ExpiresAt: now.Add(s.idempotencyTTL),
		})
	})
	if err != nil {
		return PlanResult{}, fmt.Errorf("plan capture: %w", err)
	}
	return result, nil
}

func (s *Service) MarkReady(ctx context.Context, principal auth.Principal, captureID, requestID string) (domain.CaptureSession, error) {
	return s.transition(ctx, principal, captureID, requestID, domain.CapturePlanned, domain.CaptureReady, TransitionTimes{}, "capture.ready", domain.RoleOperator)
}

func (s *Service) Start(ctx context.Context, principal auth.Principal, captureID, requestID string) (domain.CaptureSession, error) {
	now := s.clock.Now()
	return s.transition(ctx, principal, captureID, requestID, domain.CaptureReady, domain.CaptureRecording, TransitionTimes{StartedAt: &now}, "capture.start", domain.RoleOperator)
}

func (s *Service) Submit(ctx context.Context, principal auth.Principal, captureID, requestID string) (domain.CaptureSession, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return domain.CaptureSession{}, err
	}
	now := s.clock.Now()
	var updated domain.CaptureSession
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		current, err := s.repo.Find(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if current.OperatorID != principal.UserID {
			return domain.Wrap(domain.ErrForbidden, "capture.submit", "capture_session", captureID, "only the capture operator may submit", nil)
		}
		if err := current.Status.Transition(domain.CaptureProcessing); err != nil {
			return err
		}
		manifestCount, err := s.repo.CountManifests(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		unaligned, err := s.repo.CountUnalignedManifests(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if manifestCount < 2 || unaligned != 0 {
			return domain.Precondition("capture.submit", "capture_session", captureID, "at least two aligned manifests are required")
		}
		if err := s.repo.Transition(ctx, tx, principal.TenantID, captureID, current.Status, domain.CaptureProcessing, current.Version, now, TransitionTimes{SubmittedAt: &now}); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal, requestID, "capture.submit", captureID, "processing", now); err != nil {
			return err
		}
		current.Status, current.SubmittedAt, current.UpdatedAt, current.Version = domain.CaptureProcessing, &now, now, current.Version+1
		updated = current
		return nil
	})
	if err != nil {
		return domain.CaptureSession{}, fmt.Errorf("submit capture: %w", err)
	}
	return updated, nil
}

func (s *Service) Validate(ctx context.Context, principal auth.Principal, captureID, requestID string) (domain.CaptureSession, error) {
	now := s.clock.Now()
	return s.transition(ctx, principal, captureID, requestID, domain.CaptureProcessing, domain.CaptureValidated, TransitionTimes{ValidatedAt: &now}, "capture.validate", domain.RoleDataSteward, domain.RoleReviewer)
}

func (s *Service) Reject(ctx context.Context, principal auth.Principal, captureID, requestID string) (domain.CaptureSession, error) {
	return s.transition(ctx, principal, captureID, requestID, domain.CaptureProcessing, domain.CaptureRejected, TransitionTimes{}, "capture.reject", domain.RoleDataSteward, domain.RoleReviewer)
}

func (s *Service) Cancel(ctx context.Context, principal auth.Principal, captureID, requestID string) (domain.CaptureSession, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator, domain.RoleDataSteward); err != nil {
		return domain.CaptureSession{}, err
	}
	now := s.clock.Now()
	var updated domain.CaptureSession
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		current, err := s.repo.Find(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if err := current.Status.Transition(domain.CaptureCanceled); err != nil {
			return err
		}
		if principal.Role == domain.RoleOperator && current.OperatorID != principal.UserID {
			return domain.Wrap(domain.ErrForbidden, "capture.cancel", "capture_session", captureID, "operator does not own capture", nil)
		}
		lease, err := s.facilities.FindActiveLease(ctx, tx, principal.TenantID, current.RigID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if err == nil {
			if lease.CaptureID != captureID {
				return domain.Conflict("capture.cancel", "capture_session", captureID, "rig lease belongs to another capture")
			}
			if err := s.facilities.ReleaseLease(ctx, tx, principal.TenantID, lease.ID, lease.Owner, lease.Token, lease.Version, now); err != nil {
				return err
			}
		}
		if err := s.repo.Transition(ctx, tx, principal.TenantID, captureID, current.Status, domain.CaptureCanceled, current.Version, now, TransitionTimes{CanceledAt: &now}); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal, requestID, "capture.cancel", captureID, "canceled", now); err != nil {
			return err
		}
		current.Status, current.CanceledAt, current.UpdatedAt, current.Version = domain.CaptureCanceled, &now, now, current.Version+1
		updated = current
		return nil
	})
	if err != nil {
		return domain.CaptureSession{}, fmt.Errorf("cancel capture: %w", err)
	}
	return updated, nil
}

func (s *Service) Reopen(ctx context.Context, principal auth.Principal, captureID, requestID string) (domain.CaptureSession, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return domain.CaptureSession{}, err
	}
	now := s.clock.Now()
	var updated domain.CaptureSession
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		current, err := s.repo.Find(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if current.OperatorID != principal.UserID {
			return domain.Wrap(domain.ErrForbidden, "capture.reopen", "capture_session", captureID, "operator does not own capture", nil)
		}
		if err := current.Status.Transition(domain.CaptureReady); err != nil {
			return err
		}
		leaseID, err := domain.NewID("lease")
		if err != nil {
			return err
		}
		token, err := randomLeaseToken()
		if err != nil {
			return err
		}
		lease := domain.RigLease{ID: leaseID, TenantID: principal.TenantID, RigID: current.RigID, CaptureID: captureID, Owner: principal.UserID, Token: token, ExpiresAt: now.Add(s.leaseTTL), CreatedAt: now, UpdatedAt: now, Version: 1}
		if err := s.facilities.InsertLease(ctx, tx, lease); err != nil {
			return domain.Conflict("capture.reopen", "capture_rig", current.RigID, "rig is not available for rework")
		}
		if err := s.repo.Reopen(ctx, tx, principal.TenantID, captureID, current.Version, now); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal, requestID, "capture.reopen", captureID, "ready", now); err != nil {
			return err
		}
		current.Status, current.Revision, current.UpdatedAt, current.Version = domain.CaptureReady, current.Revision+1, now, current.Version+1
		current.StartedAt, current.SubmittedAt, current.ValidatedAt = nil, nil, nil
		updated = current
		return nil
	})
	if err != nil {
		return domain.CaptureSession{}, fmt.Errorf("reopen capture: %w", err)
	}
	return updated, nil
}

func (s *Service) Get(ctx context.Context, principal auth.Principal, captureID string) (domain.CaptureSession, error) {
	return s.repo.Find(ctx, s.db.SQL(), principal.TenantID, captureID)
}

func (s *Service) transition(ctx context.Context, principal auth.Principal, captureID, requestID string, from, to domain.CaptureStatus, times TransitionTimes, action string, roles ...domain.Role) (domain.CaptureSession, error) {
	if err := auth.RequireRole(principal, roles...); err != nil {
		return domain.CaptureSession{}, err
	}
	if captureID == "" || requestID == "" {
		return domain.CaptureSession{}, domain.Validation(action, "capture and request id are required")
	}
	now := s.clock.Now()
	var updated domain.CaptureSession
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		current, err := s.repo.Find(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if current.Status != from {
			return domain.Precondition(action, "capture_session", captureID, fmt.Sprintf("capture must be %s", from))
		}
		if err := current.Status.Transition(to); err != nil {
			return err
		}
		if principal.Role == domain.RoleOperator && current.OperatorID != principal.UserID {
			return domain.Wrap(domain.ErrForbidden, action, "capture_session", captureID, "operator does not own capture", nil)
		}
		if err := s.repo.Transition(ctx, tx, principal.TenantID, captureID, from, to, current.Version, now, times); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal, requestID, action, captureID, string(to), now); err != nil {
			return err
		}
		current.Status, current.UpdatedAt, current.Version = to, now, current.Version+1
		if times.StartedAt != nil {
			current.StartedAt = times.StartedAt
		}
		if times.SubmittedAt != nil {
			current.SubmittedAt = times.SubmittedAt
		}
		if times.ValidatedAt != nil {
			current.ValidatedAt = times.ValidatedAt
		}
		if times.CanceledAt != nil {
			current.CanceledAt = times.CanceledAt
		}
		updated = current
		return nil
	})
	if err != nil {
		return domain.CaptureSession{}, fmt.Errorf("%s: %w", action, err)
	}
	return updated, nil
}

func (s *Service) appendEffects(ctx context.Context, tx *sql.Tx, principal auth.Principal, requestID, action, captureID, outcome string, now time.Time) error {
	auditID, err := domain.NewID("audit")
	if err != nil {
		return err
	}
	outboxID, err := domain.NewID("event")
	if err != nil {
		return err
	}
	if err := s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: action, ObjectType: "capture_session", ObjectID: captureID, Outcome: outcome, RequestID: requestID, CreatedAt: now}); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"capture_id": captureID, "event": action, "status": outcome})
	if err != nil {
		return fmt.Errorf("encode capture event: %w", err)
	}
	return s.outbox.Enqueue(ctx, tx, domain.OutboxEvent{ID: outboxID, TenantID: principal.TenantID, Topic: action, AggregateType: "capture_session", AggregateID: captureID, Payload: string(payload), Status: domain.OutboxPending, MaxAttempts: 5, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now, Version: 1})
}

func validatePlan(input PlanInput) error {
	if input.FacilityID == "" || input.ScenarioID == "" || input.RigID == "" || strings.TrimSpace(input.ConsentRef) == "" {
		return domain.Validation("capture.plan", "facility, scenario, rig, and consent reference are required")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" || len(input.IdempotencyKey) > 128 {
		return domain.Validation("capture.plan", "idempotency key is required and must be at most 128 bytes")
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return domain.Validation("capture.plan", "request id is required")
	}
	return nil
}

func randomLeaseToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate capture lease token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
