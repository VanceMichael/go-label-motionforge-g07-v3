package dataset

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/audit"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/clock"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/outbox"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

type Service struct {
	db     *storage.Database
	repo   Repository
	audits audit.Store
	outbox outbox.Repository
	clock  clock.Clock
}

func NewService(db *storage.Database, c clock.Clock) *Service {
	return &Service{db: db, repo: Repository{}, audits: audit.Store{}, outbox: outbox.Repository{}, clock: c}
}

func (s *Service) Create(ctx context.Context, principal auth.Principal, name, requestID string) (domain.DatasetDraft, error) {
	if err := auth.RequireRole(principal, domain.RoleDataSteward); err != nil {
		return domain.DatasetDraft{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 160 || requestID == "" {
		return domain.DatasetDraft{}, domain.Validation("dataset.create", "bounded name and request id are required")
	}
	id, err := domain.NewID("dataset")
	if err != nil {
		return domain.DatasetDraft{}, err
	}
	now := s.clock.Now()
	value := domain.DatasetDraft{ID: id, TenantID: principal.TenantID, Name: name, Status: domain.DatasetStatusDraft, Revision: 1, CreatedAt: now, UpdatedAt: now, Version: 1}
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		if err := s.repo.InsertDraft(ctx, tx, value); err != nil {
			return err
		}
		return s.appendEffects(ctx, tx, principal, requestID, "dataset.create", value.ID, "draft", now)
	})
	if err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("create dataset: %w", err)
	}
	return value, nil
}

func (s *Service) AddCaptures(ctx context.Context, principal auth.Principal, datasetID, requestID string, captureIDs []string) (domain.DatasetDraft, []domain.DatasetItem, error) {
	if err := auth.RequireRole(principal, domain.RoleDataSteward); err != nil {
		return domain.DatasetDraft{}, nil, err
	}
	if datasetID == "" || requestID == "" || len(captureIDs) == 0 || len(captureIDs) > 250 {
		return domain.DatasetDraft{}, nil, domain.Validation("dataset.add_captures", "dataset, request id, and 1-250 captures are required")
	}
	seen := make(map[string]struct{}, len(captureIDs))
	for _, captureID := range captureIDs {
		if captureID == "" {
			return domain.DatasetDraft{}, nil, domain.Validation("dataset.add_captures", "capture id cannot be empty")
		}
		if _, exists := seen[captureID]; exists {
			return domain.DatasetDraft{}, nil, domain.Validation("dataset.add_captures", "capture ids must be unique within the request")
		}
		seen[captureID] = struct{}{}
	}
	now := s.clock.Now()
	var updated domain.DatasetDraft
	var items []domain.DatasetItem
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		draft, err := s.repo.FindDraft(ctx, tx, principal.TenantID, datasetID)
		if err != nil {
			return err
		}
		if draft.Status != domain.DatasetStatusDraft {
			return domain.Precondition("dataset.add_captures", "dataset_draft", datasetID, "only draft datasets accept new captures")
		}
		for _, captureID := range captureIDs {
			if err := s.repo.EligibleCapture(ctx, tx, principal.TenantID, captureID); err != nil {
				return err
			}
			id, err := domain.NewID("dataset_item")
			if err != nil {
				return err
			}
			items = append(items, domain.DatasetItem{ID: id, TenantID: principal.TenantID, DatasetID: datasetID, CaptureID: captureID, Revision: draft.Revision, CreatedAt: now})
		}
		if err := s.repo.InsertItems(ctx, tx, items); err != nil {
			return err
		}
		if err := s.repo.RefreshItemCount(ctx, tx, principal.TenantID, datasetID, draft.Version, now); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal, requestID, "dataset.members.add", datasetID, "added", now); err != nil {
			return err
		}
		draft.ItemCount += len(items)
		draft.UpdatedAt, draft.Version = now, draft.Version+1
		updated = draft
		return nil
	})
	if err != nil {
		return domain.DatasetDraft{}, nil, fmt.Errorf("add dataset captures: %w", err)
	}
	return updated, items, nil
}

func (s *Service) Freeze(ctx context.Context, principal auth.Principal, datasetID, requestID string) (domain.DatasetDraft, error) {
	if err := auth.RequireRole(principal, domain.RoleDataSteward); err != nil {
		return domain.DatasetDraft{}, err
	}
	now := s.clock.Now()
	var frozen domain.DatasetDraft
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		draft, err := s.repo.FindDraft(ctx, tx, principal.TenantID, datasetID)
		if err != nil {
			return err
		}
		if err := draft.Status.Transition(domain.DatasetStatusFrozen); err != nil {
			return err
		}
		items, err := s.repo.ListItems(ctx, tx, principal.TenantID, datasetID)
		if err != nil {
			return err
		}
		if len(items) == 0 || len(items) != draft.ItemCount {
			return domain.Precondition("dataset.freeze", "dataset_draft", datasetID, "dataset membership must be non-empty and internally consistent")
		}
		for _, item := range items {
			if err := s.repo.EligibleCapture(ctx, tx, principal.TenantID, item.CaptureID); err != nil {
				return err
			}
		}
		digest := membershipDigest(items)
		if err := s.repo.Transition(ctx, tx, principal.TenantID, datasetID, draft.Status, domain.DatasetStatusFrozen, draft.Version, digest, now); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal, requestID, "dataset.freeze", datasetID, "frozen", now); err != nil {
			return err
		}
		draft.Status, draft.Digest, draft.FrozenAt, draft.UpdatedAt, draft.Version = domain.DatasetStatusFrozen, digest, &now, now, draft.Version+1
		frozen = draft
		return nil
	})
	if err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("freeze dataset: %w", err)
	}
	return frozen, nil
}

func (s *Service) Review(ctx context.Context, principal auth.Principal, datasetID, requestID string, approve bool, reason string) (domain.DatasetDraft, error) {
	if err := auth.RequireRole(principal, domain.RoleReviewer); err != nil {
		return domain.DatasetDraft{}, err
	}
	if !approve && strings.TrimSpace(reason) == "" {
		return domain.DatasetDraft{}, domain.Validation("dataset.review", "rejection reason is required")
	}
	now := s.clock.Now()
	var reviewed domain.DatasetDraft
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		draft, err := s.repo.FindDraft(ctx, tx, principal.TenantID, datasetID)
		if err != nil {
			return err
		}
		if draft.Status != domain.DatasetStatusFrozen {
			return domain.Precondition("dataset.review", "dataset_draft", datasetID, "dataset must be frozen")
		}
		to := domain.DatasetStatusDraft
		outcome := "rejected"
		if approve {
			to, outcome = domain.DatasetStatusApproved, "approved"
		}
		if err := draft.Status.Transition(to); err != nil {
			return err
		}
		reviewID, err := domain.NewID("review")
		if err != nil {
			return err
		}
		review := domain.QualityReview{ID: reviewID, TenantID: principal.TenantID, ObjectType: "dataset_draft", ObjectID: datasetID, ReviewerID: principal.UserID, Outcome: outcome, Reason: reason, CreatedAt: now, UpdatedAt: now, Version: 1}
		if err := s.repo.InsertReview(ctx, tx, review); err != nil {
			return err
		}
		if err := s.repo.Transition(ctx, tx, principal.TenantID, datasetID, draft.Status, to, draft.Version, "", now); err != nil {
			return err
		}
		if err := s.appendEffectsWithDetail(ctx, tx, principal, requestID, "dataset.review", datasetID, outcome, reason, now); err != nil {
			return err
		}
		draft.Status, draft.UpdatedAt, draft.Version = to, now, draft.Version+1
		if !approve {
			draft.FrozenAt, draft.Digest = nil, ""
		}
		reviewed = draft
		return nil
	})
	if err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("review dataset: %w", err)
	}
	return reviewed, nil
}

func (s *Service) Publish(ctx context.Context, principal auth.Principal, datasetID, requestID string) (domain.DatasetRelease, error) {
	if err := auth.RequireRole(principal, domain.RoleDataSteward); err != nil {
		return domain.DatasetRelease{}, err
	}
	now := s.clock.Now()
	var release domain.DatasetRelease
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		draft, err := s.repo.FindDraft(ctx, tx, principal.TenantID, datasetID)
		if err != nil {
			return err
		}
		if err := draft.Status.Transition(domain.DatasetStatusPublished); err != nil {
			return err
		}
		items, err := s.repo.ListItems(ctx, tx, principal.TenantID, datasetID)
		if err != nil {
			return err
		}
		if len(items) != draft.ItemCount || membershipDigest(items) != draft.Digest {
			return domain.Conflict("dataset.publish", "dataset_draft", datasetID, "frozen membership no longer matches digest")
		}
		releaseID, err := domain.NewID("release")
		if err != nil {
			return err
		}
		release = domain.DatasetRelease{ID: releaseID, TenantID: principal.TenantID, DatasetID: datasetID, Revision: draft.Revision, Digest: draft.Digest, Status: domain.DatasetStatusPublished, PublishedAt: &now, CreatedAt: now, UpdatedAt: now, Version: 1}
		if err := s.repo.InsertRelease(ctx, tx, release, items); err != nil {
			return err
		}
		if err := s.repo.Transition(ctx, tx, principal.TenantID, datasetID, draft.Status, domain.DatasetStatusPublished, draft.Version, "", now); err != nil {
			return err
		}
		return s.appendEffects(ctx, tx, principal, requestID, "dataset.publish", releaseID, "published", now)
	})
	if err != nil {
		return domain.DatasetRelease{}, fmt.Errorf("publish dataset: %w", err)
	}
	return release, nil
}

func (s *Service) Revoke(ctx context.Context, principal auth.Principal, releaseID, requestID, reason string) (domain.DatasetRelease, error) {
	if err := auth.RequireRole(principal, domain.RoleDataSteward); err != nil {
		return domain.DatasetRelease{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return domain.DatasetRelease{}, domain.Validation("dataset.revoke", "reason is required")
	}
	now := s.clock.Now()
	var revoked domain.DatasetRelease
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		release, err := s.repo.FindRelease(ctx, tx, principal.TenantID, releaseID)
		if err != nil {
			return err
		}
		if err := release.Status.Transition(domain.DatasetStatusRevoked); err != nil {
			return err
		}
		active, err := s.repo.CountActiveJobs(ctx, tx, principal.TenantID, releaseID)
		if err != nil {
			return err
		}
		if active > 0 {
			return domain.Precondition("dataset.revoke", "dataset_release", releaseID, "release is referenced by active training jobs")
		}
		if err := s.repo.TransitionRelease(ctx, tx, principal.TenantID, releaseID, release.Status, domain.DatasetStatusRevoked, release.Version, now); err != nil {
			return err
		}
		if err := s.appendEffectsWithDetail(ctx, tx, principal, requestID, "dataset.revoke", releaseID, "revoked", reason, now); err != nil {
			return err
		}
		release.Status, release.RevokedAt, release.UpdatedAt, release.Version = domain.DatasetStatusRevoked, &now, now, release.Version+1
		revoked = release
		return nil
	})
	if err != nil {
		return domain.DatasetRelease{}, fmt.Errorf("revoke dataset release: %w", err)
	}
	return revoked, nil
}

func (s *Service) Get(ctx context.Context, principal auth.Principal, datasetID string) (domain.DatasetDraft, []domain.DatasetItem, error) {
	draft, err := s.repo.FindDraft(ctx, s.db.SQL(), principal.TenantID, datasetID)
	if err != nil {
		return domain.DatasetDraft{}, nil, err
	}
	items, err := s.repo.ListItems(ctx, s.db.SQL(), principal.TenantID, datasetID)
	if err != nil {
		return domain.DatasetDraft{}, nil, err
	}
	return draft, items, nil
}

func (s *Service) GetRelease(ctx context.Context, principal auth.Principal, releaseID string) (domain.DatasetRelease, error) {
	return s.repo.FindRelease(ctx, s.db.SQL(), principal.TenantID, releaseID)
}

func (s *Service) List(ctx context.Context, principal auth.Principal, filter ListFilter) ([]domain.DatasetDraft, int, error) {
	return s.repo.ListDrafts(ctx, s.db.SQL(), principal.TenantID, filter)
}

func (s *Service) appendEffects(ctx context.Context, tx *sql.Tx, principal auth.Principal, requestID, action, objectID, outcome string, now time.Time) error {
	return s.appendEffectsWithDetail(ctx, tx, principal, requestID, action, objectID, outcome, "", now)
}

func (s *Service) appendEffectsWithDetail(ctx context.Context, tx *sql.Tx, principal auth.Principal, requestID, action, objectID, outcome, detail string, now time.Time) error {
	auditID, err := domain.NewID("audit")
	if err != nil {
		return err
	}
	eventID, err := domain.NewID("event")
	if err != nil {
		return err
	}
	if err := s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: action, ObjectType: "dataset", ObjectID: objectID, Outcome: outcome, RequestID: requestID, Detail: detail, CreatedAt: now}); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"object_id": objectID, "event": action, "outcome": outcome})
	return s.outbox.Enqueue(ctx, tx, domain.OutboxEvent{ID: eventID, TenantID: principal.TenantID, Topic: action, AggregateType: "dataset", AggregateID: objectID, Payload: string(payload), Status: domain.OutboxPending, MaxAttempts: 5, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now, Version: 1})
}
