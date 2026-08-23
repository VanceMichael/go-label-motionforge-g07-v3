package capture

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

type Repository struct{}

func (Repository) Insert(ctx context.Context, q storage.Queryer, value domain.CaptureSession) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO capture_sessions(
			id, tenant_id, facility_id, scenario_id, rig_id, operator_id,
			status, revision, consent_ref, started_at, submitted_at,
			validated_at, canceled_at, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.TenantID, value.FacilityID, value.ScenarioID, value.RigID,
		value.OperatorID, value.Status, value.Revision, value.ConsentRef,
		storage.NullableTime(value.StartedAt), storage.NullableTime(value.SubmittedAt),
		storage.NullableTime(value.ValidatedAt), storage.NullableTime(value.CanceledAt),
		storage.FormatTime(value.CreatedAt), storage.FormatTime(value.UpdatedAt), value.Version)
	if err != nil {
		return fmt.Errorf("insert capture session: %w", err)
	}
	return nil
}

func (Repository) Find(ctx context.Context, q storage.Queryer, tenantID, id string) (domain.CaptureSession, error) {
	var value domain.CaptureSession
	var started, submitted, validated, canceled sql.NullString
	var created, updated string
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, facility_id, scenario_id, rig_id, operator_id,
		       status, revision, consent_ref, started_at, submitted_at,
		       validated_at, canceled_at, created_at, updated_at, version
		FROM capture_sessions WHERE tenant_id = ? AND id = ?`, tenantID, id,
	).Scan(&value.ID, &value.TenantID, &value.FacilityID, &value.ScenarioID,
		&value.RigID, &value.OperatorID, &value.Status, &value.Revision,
		&value.ConsentRef, &started, &submitted, &validated, &canceled,
		&created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CaptureSession{}, domain.NotFound("capture.find", "capture_session", id)
	}
	if err != nil {
		return domain.CaptureSession{}, fmt.Errorf("find capture session: %w", err)
	}
	var parseErr error
	if value.StartedAt, parseErr = storage.ScanNullableTime(started); parseErr != nil {
		return domain.CaptureSession{}, fmt.Errorf("parse capture started_at: %w", parseErr)
	}
	if value.SubmittedAt, parseErr = storage.ScanNullableTime(submitted); parseErr != nil {
		return domain.CaptureSession{}, fmt.Errorf("parse capture submitted_at: %w", parseErr)
	}
	if value.ValidatedAt, parseErr = storage.ScanNullableTime(validated); parseErr != nil {
		return domain.CaptureSession{}, fmt.Errorf("parse capture validated_at: %w", parseErr)
	}
	if value.CanceledAt, parseErr = storage.ScanNullableTime(canceled); parseErr != nil {
		return domain.CaptureSession{}, fmt.Errorf("parse capture canceled_at: %w", parseErr)
	}
	if value.CreatedAt, parseErr = storage.ParseTime(created); parseErr != nil {
		return domain.CaptureSession{}, fmt.Errorf("parse capture created_at: %w", parseErr)
	}
	if value.UpdatedAt, parseErr = storage.ParseTime(updated); parseErr != nil {
		return domain.CaptureSession{}, fmt.Errorf("parse capture updated_at: %w", parseErr)
	}
	return value, nil
}

type TransitionTimes struct {
	StartedAt   *time.Time
	SubmittedAt *time.Time
	ValidatedAt *time.Time
	CanceledAt  *time.Time
}

func (Repository) Transition(ctx context.Context, q storage.Queryer, tenantID, id string, from, to domain.CaptureStatus, version int64, now time.Time, times TransitionTimes) error {
	result, err := q.ExecContext(ctx, `
		UPDATE capture_sessions
		SET status = ?, started_at = COALESCE(?, started_at),
		    submitted_at = COALESCE(?, submitted_at),
		    validated_at = COALESCE(?, validated_at),
		    canceled_at = COALESCE(?, canceled_at),
		    updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND status = ? AND version = ?`,
		to, storage.NullableTime(times.StartedAt), storage.NullableTime(times.SubmittedAt),
		storage.NullableTime(times.ValidatedAt), storage.NullableTime(times.CanceledAt),
		storage.FormatTime(now), tenantID, id, from, version)
	if err != nil {
		return fmt.Errorf("transition capture session: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read capture transition result: %w", err)
	}
	if changed != 1 {
		return domain.Conflict("capture.transition", "capture_session", id, "state or version changed")
	}
	return nil
}

func (Repository) Reopen(ctx context.Context, q storage.Queryer, tenantID, id string, version int64, now time.Time) error {
	result, err := q.ExecContext(ctx, `
		UPDATE capture_sessions
		SET status = 'ready', revision = revision + 1,
		    started_at = NULL, submitted_at = NULL, validated_at = NULL,
		    updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND status = 'rejected' AND version = ?`,
		storage.FormatTime(now), tenantID, id, version)
	if err != nil {
		return fmt.Errorf("reopen capture session: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read reopen result: %w", err)
	}
	if changed != 1 {
		return domain.Conflict("capture.reopen", "capture_session", id, "state or version changed")
	}
	return nil
}

func (Repository) CountUnalignedManifests(ctx context.Context, q storage.Queryer, tenantID, captureID string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stream_manifests
		WHERE tenant_id = ? AND capture_id = ? AND status <> 'aligned'`, tenantID, captureID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unaligned manifests: %w", err)
	}
	return count, nil
}

func (Repository) CountManifests(ctx context.Context, q storage.Queryer, tenantID, captureID string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM stream_manifests WHERE tenant_id = ? AND capture_id = ?`, tenantID, captureID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count capture manifests: %w", err)
	}
	return count, nil
}
