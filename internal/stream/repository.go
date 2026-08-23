package stream

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

type Repository struct{}

func (Repository) InsertManifest(ctx context.Context, q storage.Queryer, value domain.StreamManifest) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO stream_manifests(
			id, tenant_id, capture_id, kind, status, segment_count,
			first_nanos, last_nanos, digest, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.TenantID, value.CaptureID, value.Kind, value.Status,
		value.SegmentCount, value.FirstNanos, value.LastNanos, value.Digest,
		storage.FormatTime(value.CreatedAt), storage.FormatTime(value.UpdatedAt), value.Version)
	if err != nil {
		return fmt.Errorf("insert stream manifest: %w", err)
	}
	return nil
}

func (Repository) FindManifest(ctx context.Context, q storage.Queryer, tenantID, id string) (domain.StreamManifest, error) {
	return scanManifest(q.QueryRowContext(ctx, `
		SELECT id, tenant_id, capture_id, kind, status, segment_count,
		       first_nanos, last_nanos, digest, created_at, updated_at, version
		FROM stream_manifests WHERE tenant_id = ? AND id = ?`, tenantID, id), id)
}

func (Repository) FindManifestByKind(ctx context.Context, q storage.Queryer, tenantID, captureID string, kind domain.StreamKind) (domain.StreamManifest, error) {
	return scanManifest(q.QueryRowContext(ctx, `
		SELECT id, tenant_id, capture_id, kind, status, segment_count,
		       first_nanos, last_nanos, digest, created_at, updated_at, version
		FROM stream_manifests WHERE tenant_id = ? AND capture_id = ? AND kind = ?`, tenantID, captureID, kind), captureID+":"+string(kind))
}

func (Repository) ListManifests(ctx context.Context, q storage.Queryer, tenantID, captureID string) ([]domain.StreamManifest, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, tenant_id, capture_id, kind, status, segment_count,
		       first_nanos, last_nanos, digest, created_at, updated_at, version
		FROM stream_manifests WHERE tenant_id = ? AND capture_id = ?
		ORDER BY kind ASC`, tenantID, captureID)
	if err != nil {
		return nil, fmt.Errorf("list stream manifests: %w", err)
	}
	defer rows.Close()
	values := make([]domain.StreamManifest, 0)
	for rows.Next() {
		value, err := scanManifest(rows, "")
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stream manifests: %w", err)
	}
	return values, nil
}

func scanManifest(row scanner, id string) (domain.StreamManifest, error) {
	var value domain.StreamManifest
	var created, updated string
	err := row.Scan(&value.ID, &value.TenantID, &value.CaptureID, &value.Kind,
		&value.Status, &value.SegmentCount, &value.FirstNanos, &value.LastNanos,
		&value.Digest, &created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.StreamManifest{}, domain.NotFound("stream.find_manifest", "stream_manifest", id)
	}
	if err != nil {
		return domain.StreamManifest{}, fmt.Errorf("scan stream manifest: %w", err)
	}
	var parseErr error
	if value.CreatedAt, parseErr = storage.ParseTime(created); parseErr != nil {
		return domain.StreamManifest{}, fmt.Errorf("parse manifest created_at: %w", parseErr)
	}
	if value.UpdatedAt, parseErr = storage.ParseTime(updated); parseErr != nil {
		return domain.StreamManifest{}, fmt.Errorf("parse manifest updated_at: %w", parseErr)
	}
	return value, nil
}

func (Repository) FindSegmentByKey(ctx context.Context, q storage.Queryer, tenantID, manifestID, key string) (domain.StreamSegment, bool, error) {
	var value domain.StreamSegment
	var created string
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, manifest_id, sequence, start_nanos, end_nanos,
		       object_uri, checksum, idempotency_key, created_at
		FROM stream_segments
		WHERE tenant_id = ? AND manifest_id = ? AND idempotency_key = ?`, tenantID, manifestID, key,
	).Scan(&value.ID, &value.TenantID, &value.ManifestID, &value.Sequence,
		&value.StartNanos, &value.EndNanos, &value.ObjectURI, &value.Checksum,
		&value.IdempotencyKey, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.StreamSegment{}, false, nil
	}
	if err != nil {
		return domain.StreamSegment{}, false, fmt.Errorf("find stream segment by key: %w", err)
	}
	value.CreatedAt, err = storage.ParseTime(created)
	if err != nil {
		return domain.StreamSegment{}, false, fmt.Errorf("parse segment created_at: %w", err)
	}
	return value, true, nil
}

func (Repository) InsertSegment(ctx context.Context, q storage.Queryer, value domain.StreamSegment) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO stream_segments(
			id, tenant_id, manifest_id, sequence, start_nanos, end_nanos,
			object_uri, checksum, idempotency_key, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.TenantID,
		value.ManifestID, value.Sequence, value.StartNanos, value.EndNanos,
		value.ObjectURI, value.Checksum, value.IdempotencyKey, storage.FormatTime(value.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert stream segment: %w", err)
	}
	return nil
}

func (Repository) ListSegments(ctx context.Context, q storage.Queryer, tenantID, manifestID string) ([]domain.StreamSegment, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, tenant_id, manifest_id, sequence, start_nanos, end_nanos,
		       object_uri, checksum, idempotency_key, created_at
		FROM stream_segments WHERE tenant_id = ? AND manifest_id = ?
		ORDER BY sequence ASC, id ASC`, tenantID, manifestID)
	if err != nil {
		return nil, fmt.Errorf("list stream segments: %w", err)
	}
	defer rows.Close()
	values := make([]domain.StreamSegment, 0)
	for rows.Next() {
		var value domain.StreamSegment
		var created string
		if err := rows.Scan(&value.ID, &value.TenantID, &value.ManifestID,
			&value.Sequence, &value.StartNanos, &value.EndNanos, &value.ObjectURI,
			&value.Checksum, &value.IdempotencyKey, &created); err != nil {
			return nil, fmt.Errorf("scan stream segment: %w", err)
		}
		parsed, err := storage.ParseTime(created)
		if err != nil {
			return nil, fmt.Errorf("parse segment created_at: %w", err)
		}
		value.CreatedAt = parsed
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stream segments: %w", err)
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Sequence == values[j].Sequence {
			return values[i].ID < values[j].ID
		}
		return values[i].Sequence < values[j].Sequence
	})
	return values, nil
}

func (Repository) RefreshAggregate(ctx context.Context, q storage.Queryer, tenantID, manifestID string, expectedVersion int64, now string) error {
	result, err := q.ExecContext(ctx, `
		UPDATE stream_manifests
		SET segment_count = (
		        SELECT COUNT(*) FROM stream_segments
		        WHERE tenant_id = ? AND manifest_id = ?
		    ),
		    first_nanos = COALESCE((
		        SELECT MIN(start_nanos) FROM stream_segments
		        WHERE tenant_id = ? AND manifest_id = ?
		    ), 0),
		    last_nanos = COALESCE((
		        SELECT MAX(end_nanos) FROM stream_segments
		        WHERE tenant_id = ? AND manifest_id = ?
		    ), 0),
		    updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND status = 'open' AND version = ?`,
		tenantID, manifestID, tenantID, manifestID, tenantID, manifestID,
		now, tenantID, manifestID, expectedVersion)
	if err != nil {
		return fmt.Errorf("refresh stream manifest aggregate: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read manifest aggregate result: %w", err)
	}
	if changed != 1 {
		return domain.Conflict("stream.refresh_manifest", "stream_manifest", manifestID, "manifest changed or was sealed")
	}
	return nil
}

func (Repository) TransitionManifest(ctx context.Context, q storage.Queryer, tenantID, id string, from, to domain.ManifestStatus, version int64, digest, now string) error {
	result, err := q.ExecContext(ctx, `
		UPDATE stream_manifests
		SET status = ?, digest = CASE WHEN ? = '' THEN digest ELSE ? END,
		    updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ?`,
		to, digest, digest, now, tenantID, id)
	if err != nil {
		return fmt.Errorf("transition stream manifest: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read manifest transition result: %w", err)
	}
	if changed != 1 {
		return domain.Conflict("stream.transition_manifest", "stream_manifest", id, "state or version changed")
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}
