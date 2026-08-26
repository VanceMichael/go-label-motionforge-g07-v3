package stream

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/audit"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/clock"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/outbox"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

type Service struct {
	db       *storage.Database
	repo     Repository
	captures capture.Repository
	audits   audit.Store
	outbox   outbox.Repository
	clock    clock.Clock
}

type SegmentInput struct {
	Sequence       int
	StartNanos     int64
	EndNanos       int64
	ObjectURI      string
	Checksum       string
	IdempotencyKey string
}

type AppendResult struct {
	Segments []domain.StreamSegment
	Inserted int
	Replayed int
}

func NewService(db *storage.Database, c clock.Clock) *Service {
	return &Service{db: db, repo: Repository{}, captures: capture.Repository{}, audits: audit.Store{}, outbox: outbox.Repository{}, clock: c}
}

func (s *Service) OpenManifest(ctx context.Context, principal auth.Principal, captureID string, kind domain.StreamKind, requestID string) (domain.StreamManifest, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return domain.StreamManifest{}, err
	}
	if captureID == "" || !kind.Valid() || requestID == "" {
		return domain.StreamManifest{}, domain.Validation("stream.open_manifest", "capture, stream kind, and request id are required")
	}
	id, err := domain.NewID("manifest")
	if err != nil {
		return domain.StreamManifest{}, err
	}
	auditID, err := domain.NewID("audit")
	if err != nil {
		return domain.StreamManifest{}, err
	}
	now := s.clock.Now()
	manifest := domain.StreamManifest{ID: id, TenantID: principal.TenantID, CaptureID: captureID, Kind: kind, Status: domain.ManifestOpen, CreatedAt: now, UpdatedAt: now, Version: 1}
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		captureValue, err := s.captures.Find(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if captureValue.OperatorID != principal.UserID {
			return domain.Wrap(domain.ErrForbidden, "stream.open_manifest", "capture_session", captureID, "operator does not own capture", nil)
		}
		if captureValue.Status != domain.CaptureReady && captureValue.Status != domain.CaptureRecording {
			return domain.Precondition("stream.open_manifest", "capture_session", captureID, "capture must be ready or recording")
		}
		if _, err := s.repo.FindManifestByKind(ctx, tx, principal.TenantID, captureID, kind); err == nil {
			return domain.Conflict("stream.open_manifest", "stream_manifest", captureID+":"+string(kind), "stream kind already exists")
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if err := s.repo.InsertManifest(ctx, tx, manifest); err != nil {
			return err
		}
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "stream.open", ObjectType: "stream_manifest", ObjectID: manifest.ID, Outcome: "open", RequestID: requestID, CreatedAt: now})
	})
	if err != nil {
		return domain.StreamManifest{}, fmt.Errorf("open stream manifest: %w", err)
	}
	return manifest, nil
}

func (s *Service) Append(ctx context.Context, principal auth.Principal, manifestID, requestID string, inputs []SegmentInput) (AppendResult, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return AppendResult{}, err
	}
	if manifestID == "" || requestID == "" || len(inputs) == 0 || len(inputs) > 250 {
		return AppendResult{}, domain.Validation("stream.append", "manifest, request id, and 1-250 segments are required")
	}
	if err := validateInputs(inputs); err != nil {
		return AppendResult{}, err
	}
	now := s.clock.Now()
	result := AppendResult{Segments: make([]domain.StreamSegment, 0, len(inputs))}
	// Persist the first item before the batch transaction.  A later item
	// failure therefore leaves a prefix of the batch visible.
	firstID, err := domain.NewID("segment")
	if err != nil {
		return AppendResult{}, err
	}
	first := inputs[0]
	if err := s.repo.InsertSegment(ctx, s.db.SQL(), domain.StreamSegment{ID: firstID, TenantID: principal.TenantID, ManifestID: manifestID, Sequence: first.Sequence, StartNanos: first.StartNanos, EndNanos: first.EndNanos, ObjectURI: first.ObjectURI, Checksum: strings.ToLower(first.Checksum), IdempotencyKey: first.IdempotencyKey, CreatedAt: now}); err != nil {
		return AppendResult{}, err
	}
	result.Inserted = 1
	result.Segments = append(result.Segments, domain.StreamSegment{ID: firstID, TenantID: principal.TenantID, ManifestID: manifestID, Sequence: first.Sequence, StartNanos: first.StartNanos, EndNanos: first.EndNanos, ObjectURI: first.ObjectURI, Checksum: strings.ToLower(first.Checksum), IdempotencyKey: first.IdempotencyKey, CreatedAt: now})
	inputs = inputs[1:]
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		manifest, err := s.repo.FindManifest(ctx, tx, principal.TenantID, manifestID)
		if err != nil {
			return err
		}
		captureValue, err := s.captures.Find(ctx, tx, principal.TenantID, manifest.CaptureID)
		if err != nil {
			return err
		}
		if captureValue.OperatorID != principal.UserID {
			return domain.Wrap(domain.ErrForbidden, "stream.append", "capture_session", captureValue.ID, "operator does not own capture", nil)
		}
		if captureValue.Status != domain.CaptureRecording || manifest.Status != domain.ManifestOpen {
			return domain.Precondition("stream.append", "stream_manifest", manifestID, "capture must be recording and manifest open")
		}
		for _, input := range inputs {
			existing, found, err := s.repo.FindSegmentByKey(ctx, tx, principal.TenantID, manifestID, input.IdempotencyKey)
			if err != nil {
				return err
			}
			if found {
				if !sameSegment(existing, input) {
					return domain.Wrap(domain.ErrIdempotencyConflict, "stream.append", "stream_segment", existing.ID, "idempotency key represents different segment data", nil)
				}
				result.Segments = append(result.Segments, existing)
				result.Replayed++
				continue
			}
			id, err := domain.NewID("segment")
			if err != nil {
				return err
			}
			segment := domain.StreamSegment{ID: id, TenantID: principal.TenantID, ManifestID: manifestID, Sequence: input.Sequence, StartNanos: input.StartNanos, EndNanos: input.EndNanos, ObjectURI: input.ObjectURI, Checksum: strings.ToLower(input.Checksum), IdempotencyKey: input.IdempotencyKey, CreatedAt: now}
			if err := s.repo.InsertSegment(ctx, tx, segment); err != nil {
				return err
			}
			result.Segments = append(result.Segments, segment)
			result.Inserted++
		}
		if result.Inserted > 0 {
			if err := s.repo.RefreshAggregate(ctx, tx, principal.TenantID, manifestID, manifest.Version, storage.FormatTime(now)); err != nil {
				return err
			}
		}
		auditID, err := domain.NewID("audit")
		if err != nil {
			return err
		}
		detail, _ := json.Marshal(map[string]int{"inserted": result.Inserted, "replayed": result.Replayed})
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "stream.append", ObjectType: "stream_manifest", ObjectID: manifestID, Outcome: "accepted", RequestID: requestID, Detail: string(detail), CreatedAt: now})
	})
	if err != nil {
		return AppendResult{}, fmt.Errorf("append stream segments: %w", err)
	}
	return result, nil
}

func (s *Service) Seal(ctx context.Context, principal auth.Principal, manifestID, requestID string) (domain.StreamManifest, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return domain.StreamManifest{}, err
	}
	now := s.clock.Now()
	var sealed domain.StreamManifest
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		manifest, err := s.repo.FindManifest(ctx, tx, principal.TenantID, manifestID)
		if err != nil {
			return err
		}
		captureValue, err := s.captures.Find(ctx, tx, principal.TenantID, manifest.CaptureID)
		if err != nil {
			return err
		}
		if captureValue.Status != domain.CaptureRecording || captureValue.OperatorID != principal.UserID {
			return domain.Precondition("stream.seal", "capture_session", captureValue.ID, "owned capture must be recording")
		}
		if err := manifest.Status.Transition(domain.ManifestSealed); err != nil {
			return err
		}
		segments, err := s.repo.ListSegments(ctx, tx, principal.TenantID, manifestID)
		if err != nil {
			return err
		}
		if len(segments) == 0 {
			return domain.Precondition("stream.seal", "stream_manifest", manifestID, "manifest has no segments")
		}
		if err := validateContinuity(segments); err != nil {
			return err
		}
		digest := segmentDigest(segments)
		if err := s.repo.TransitionManifest(ctx, tx, principal.TenantID, manifestID, manifest.Status, domain.ManifestSealed, manifest.Version, digest, storage.FormatTime(now)); err != nil {
			return err
		}
		auditID, err := domain.NewID("audit")
		if err != nil {
			return err
		}
		if err := s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "stream.seal", ObjectType: "stream_manifest", ObjectID: manifestID, Outcome: "sealed", RequestID: requestID, Detail: digest, CreatedAt: now}); err != nil {
			return err
		}
		manifest.Status, manifest.Digest, manifest.UpdatedAt, manifest.Version = domain.ManifestSealed, digest, now, manifest.Version+1
		sealed = manifest
		return nil
	})
	if err != nil {
		return domain.StreamManifest{}, fmt.Errorf("seal stream manifest: %w", err)
	}
	return sealed, nil
}

func (s *Service) AlignCapture(ctx context.Context, principal auth.Principal, captureID, requestID string, tolerance time.Duration) ([]domain.StreamManifest, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator, domain.RoleReviewer); err != nil {
		return nil, err
	}
	if tolerance < 0 || tolerance > 5*time.Second {
		return nil, domain.Validation("stream.align", "tolerance must be between zero and five seconds")
	}
	now := s.clock.Now()
	var aligned []domain.StreamManifest
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		captureValue, err := s.captures.Find(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if captureValue.Status != domain.CaptureRecording {
			return domain.Precondition("stream.align", "capture_session", captureID, "capture must be recording")
		}
		if principal.Role == domain.RoleOperator && captureValue.OperatorID != principal.UserID {
			return domain.Wrap(domain.ErrForbidden, "stream.align", "capture_session", captureID, "operator does not own capture", nil)
		}
		manifests, err := s.repo.ListManifests(ctx, tx, principal.TenantID, captureID)
		if err != nil {
			return err
		}
		if len(manifests) < 2 {
			return domain.Precondition("stream.align", "capture_session", captureID, "at least two streams are required")
		}
		if err := validateAlignment(manifests, tolerance); err != nil {
			return err
		}
		for i := range manifests {
			manifest := &manifests[i]
			if err := manifest.Status.Transition(domain.ManifestAligned); err != nil {
				return err
			}
			if err := s.repo.TransitionManifest(ctx, tx, principal.TenantID, manifest.ID, domain.ManifestSealed, domain.ManifestAligned, manifest.Version, "", storage.FormatTime(now)); err != nil {
				return err
			}
			manifest.Status, manifest.UpdatedAt, manifest.Version = domain.ManifestAligned, now, manifest.Version+1
		}
		auditID, err := domain.NewID("audit")
		if err != nil {
			return err
		}
		outboxID, err := domain.NewID("event")
		if err != nil {
			return err
		}
		if err := s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "stream.align", ObjectType: "capture_session", ObjectID: captureID, Outcome: "aligned", RequestID: requestID, CreatedAt: now}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"capture_id": captureID, "manifest_count": len(manifests)})
		if err := s.outbox.Enqueue(ctx, tx, domain.OutboxEvent{ID: outboxID, TenantID: principal.TenantID, Topic: "stream.aligned", AggregateType: "capture_session", AggregateID: captureID, Payload: string(payload), Status: domain.OutboxPending, MaxAttempts: 5, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now, Version: 1}); err != nil {
			return err
		}
		aligned = manifests
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("align capture streams: %w", err)
	}
	return aligned, nil
}

func (s *Service) List(ctx context.Context, principal auth.Principal, captureID string) ([]domain.StreamManifest, error) {
	if _, err := s.captures.Find(ctx, s.db.SQL(), principal.TenantID, captureID); err != nil {
		return nil, err
	}
	return s.repo.ListManifests(ctx, s.db.SQL(), principal.TenantID, captureID)
}

func validateInputs(inputs []SegmentInput) error {
	sequences := make(map[int]struct{}, len(inputs))
	keys := make(map[string]struct{}, len(inputs))
	for i, input := range inputs {
		if input.Sequence < 0 || input.StartNanos < 0 || input.EndNanos <= input.StartNanos {
			return domain.Validation("stream.append", fmt.Sprintf("segment %d has an invalid sequence or time range", i))
		}
		if strings.TrimSpace(input.ObjectURI) == "" || strings.TrimSpace(input.Checksum) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
			return domain.Validation("stream.append", fmt.Sprintf("segment %d requires object URI, checksum, and idempotency key", i))
		}
		if len(input.Checksum) != 64 {
			return domain.Validation("stream.append", fmt.Sprintf("segment %d checksum must be a SHA-256 hex digest", i))
		}
		if _, err := hex.DecodeString(input.Checksum); err != nil {
			return domain.Validation("stream.append", fmt.Sprintf("segment %d checksum is not hexadecimal", i))
		}
		if _, exists := sequences[input.Sequence]; exists {
			return domain.Validation("stream.append", fmt.Sprintf("segment %d repeats a sequence within the batch", i))
		}
		if _, exists := keys[input.IdempotencyKey]; exists {
			return domain.Validation("stream.append", fmt.Sprintf("segment %d repeats an idempotency key within the batch", i))
		}
		sequences[input.Sequence], keys[input.IdempotencyKey] = struct{}{}, struct{}{}
	}
	return nil
}

func sameSegment(existing domain.StreamSegment, input SegmentInput) bool {
	return existing.Sequence == input.Sequence && existing.StartNanos == input.StartNanos &&
		existing.EndNanos == input.EndNanos && existing.ObjectURI == input.ObjectURI &&
		existing.Checksum == strings.ToLower(input.Checksum)
}

func validateContinuity(segments []domain.StreamSegment) error {
	sorted := append([]domain.StreamSegment(nil), segments...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Sequence < sorted[j].Sequence })
	for index, segment := range sorted {
		if segment.Sequence != index {
			return domain.Precondition("stream.seal", "stream_manifest", segment.ManifestID, "segment sequence must be contiguous from zero")
		}
		if index > 0 && segment.StartNanos < sorted[index-1].EndNanos {
			return domain.Precondition("stream.seal", "stream_manifest", segment.ManifestID, "segment time ranges overlap")
		}
	}
	return nil
}

func segmentDigest(segments []domain.StreamSegment) string {
	h := sha256.New()
	for _, segment := range segments {
		fmt.Fprintf(h, "%d\x00%d\x00%d\x00%s\x00%s\n", segment.Sequence, segment.StartNanos, segment.EndNanos, segment.ObjectURI, segment.Checksum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func validateAlignment(manifests []domain.StreamManifest, tolerance time.Duration) error {
	maxStart := int64(0)
	minEnd := int64(^uint64(0) >> 1)
	for _, manifest := range manifests {
		if manifest.Status != domain.ManifestSealed || manifest.SegmentCount == 0 {
			return domain.Precondition("stream.align", "stream_manifest", manifest.ID, "all manifests must be non-empty and sealed")
		}
		if manifest.FirstNanos > maxStart {
			maxStart = manifest.FirstNanos
		}
		if manifest.LastNanos < minEnd {
			minEnd = manifest.LastNanos
		}
	}
	toleranceNanos := tolerance.Nanoseconds()
	if maxStart-minEnd > toleranceNanos {
		return domain.Precondition("stream.align", "capture_session", manifests[0].CaptureID, "stream time ranges do not overlap within tolerance")
	}
	return nil
}
