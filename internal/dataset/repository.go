package dataset

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

type Repository struct{}

type ListFilter struct {
	Statuses []domain.DatasetStatus
	Search   string
	Limit    int
	Offset   int
}

func (Repository) InsertDraft(ctx context.Context, q storage.Queryer, value domain.DatasetDraft) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO dataset_drafts(
			id, tenant_id, name, status, revision, digest, item_count,
			frozen_at, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.TenantID,
		value.Name, value.Status, value.Revision, value.Digest, value.ItemCount,
		storage.NullableTime(value.FrozenAt), storage.FormatTime(value.CreatedAt),
		storage.FormatTime(value.UpdatedAt), value.Version)
	if err != nil {
		return fmt.Errorf("insert dataset draft: %w", err)
	}
	return nil
}

func (Repository) FindDraft(ctx context.Context, q storage.Queryer, tenantID, id string) (domain.DatasetDraft, error) {
	var value domain.DatasetDraft
	var frozen sql.NullString
	var created, updated string
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, status, revision, digest, item_count,
		       frozen_at, created_at, updated_at, version
		FROM dataset_drafts WHERE tenant_id = ? AND id = ?`, tenantID, id,
	).Scan(&value.ID, &value.TenantID, &value.Name, &value.Status, &value.Revision,
		&value.Digest, &value.ItemCount, &frozen, &created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DatasetDraft{}, domain.NotFound("dataset.find", "dataset_draft", id)
	}
	if err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("find dataset draft: %w", err)
	}
	var parseErr error
	if value.FrozenAt, parseErr = storage.ScanNullableTime(frozen); parseErr != nil {
		return domain.DatasetDraft{}, fmt.Errorf("parse dataset frozen_at: %w", parseErr)
	}
	if value.CreatedAt, parseErr = storage.ParseTime(created); parseErr != nil {
		return domain.DatasetDraft{}, fmt.Errorf("parse dataset created_at: %w", parseErr)
	}
	if value.UpdatedAt, parseErr = storage.ParseTime(updated); parseErr != nil {
		return domain.DatasetDraft{}, fmt.Errorf("parse dataset updated_at: %w", parseErr)
	}
	return value, nil
}

func (Repository) ListDrafts(ctx context.Context, q storage.Queryer, tenantID string, filter ListFilter) ([]domain.DatasetDraft, int, error) {
	if filter.Limit < 1 || filter.Limit > 200 || filter.Offset < 0 {
		return nil, 0, domain.Validation("dataset.list", "limit must be 1-200 and offset non-negative")
	}
	where := []string{"tenant_id = ?"}
	args := []any{tenantID}
	if filter.Search != "" {
		where = append(where, "LOWER(name) LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escapeLike(strings.ToLower(filter.Search))+"%")
	}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for i, status := range filter.Statuses {
			placeholders[i] = "?"
			args = append(args, status)
		}
		where = append(where, "status IN ("+strings.Join(placeholders, ",")+")")
	}
	predicate := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM dataset_drafts WHERE `+predicate, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count dataset drafts: %w", err)
	}
	queryArgs := append(append([]any(nil), args...), filter.Limit, filter.Offset)
	rows, err := q.QueryContext(ctx, `
		SELECT id, tenant_id, name, status, revision, digest, item_count,
		       frozen_at, created_at, updated_at, version
		FROM dataset_drafts WHERE `+predicate+`
		ORDER BY updated_at DESC, id ASC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list dataset drafts: %w", err)
	}
	defer rows.Close()
	values := make([]domain.DatasetDraft, 0)
	for rows.Next() {
		value, err := scanDraft(rows)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate dataset drafts: %w", err)
	}
	return values, total, nil
}

func scanDraft(row scanner) (domain.DatasetDraft, error) {
	var value domain.DatasetDraft
	var frozen sql.NullString
	var created, updated string
	if err := row.Scan(&value.ID, &value.TenantID, &value.Name, &value.Status,
		&value.Revision, &value.Digest, &value.ItemCount, &frozen,
		&created, &updated, &value.Version); err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("scan dataset draft: %w", err)
	}
	var err error
	if value.FrozenAt, err = storage.ScanNullableTime(frozen); err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("parse dataset frozen_at: %w", err)
	}
	if value.CreatedAt, err = storage.ParseTime(created); err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("parse dataset created_at: %w", err)
	}
	if value.UpdatedAt, err = storage.ParseTime(updated); err != nil {
		return domain.DatasetDraft{}, fmt.Errorf("parse dataset updated_at: %w", err)
	}
	return value, nil
}

func (Repository) EligibleCapture(ctx context.Context, q storage.Queryer, tenantID, captureID string) error {
	var status domain.CaptureStatus
	var consent string
	err := q.QueryRowContext(ctx, `
		SELECT status, consent_ref FROM capture_sessions
		WHERE tenant_id = ? AND id = ?`, tenantID, captureID).Scan(&status, &consent)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NotFound("dataset.eligible_capture", "capture_session", captureID)
	}
	if err != nil {
		return fmt.Errorf("load dataset capture eligibility: %w", err)
	}
	if status != domain.CaptureValidated || strings.TrimSpace(consent) == "" {
		return domain.Precondition("dataset.eligible_capture", "capture_session", captureID, "capture must be validated and consented")
	}
	var accepted int
	err = q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM annotation_batches
		WHERE tenant_id = ? AND capture_id = ? AND status = 'accepted'`, tenantID, captureID).Scan(&accepted)
	if err != nil {
		return fmt.Errorf("count accepted annotation batches: %w", err)
	}
	if accepted == 0 {
		return domain.Precondition("dataset.eligible_capture", "capture_session", captureID, "accepted annotation is required")
	}
	return nil
}

func (Repository) InsertItems(ctx context.Context, q storage.Queryer, items []domain.DatasetItem) error {
	for _, item := range items {
		if _, err := q.ExecContext(ctx, `
			INSERT OR IGNORE INTO dataset_items(id, tenant_id, dataset_id, capture_id, revision, created_at)
			VALUES(?, ?, ?, ?, ?, ?)`, item.ID, item.TenantID, item.DatasetID,
			item.CaptureID, item.Revision, storage.FormatTime(item.CreatedAt)); err != nil {
			return fmt.Errorf("insert dataset item %s: %w", item.ID, err)
		}
	}
	return nil
}

func (Repository) ListItems(ctx context.Context, q storage.Queryer, tenantID, datasetID string) ([]domain.DatasetItem, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, tenant_id, dataset_id, capture_id, revision, created_at
		FROM dataset_items WHERE tenant_id = ? AND dataset_id = ?
		ORDER BY capture_id ASC, id ASC`, tenantID, datasetID)
	if err != nil {
		return nil, fmt.Errorf("list dataset items: %w", err)
	}
	defer rows.Close()
	values := make([]domain.DatasetItem, 0)
	for rows.Next() {
		var value domain.DatasetItem
		var created string
		if err := rows.Scan(&value.ID, &value.TenantID, &value.DatasetID,
			&value.CaptureID, &value.Revision, &created); err != nil {
			return nil, fmt.Errorf("scan dataset item: %w", err)
		}
		parsed, err := storage.ParseTime(created)
		if err != nil {
			return nil, fmt.Errorf("parse dataset item created_at: %w", err)
		}
		value.CreatedAt = parsed
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dataset items: %w", err)
	}
	return values, nil
}

func (Repository) RefreshItemCount(ctx context.Context, q storage.Queryer, tenantID, datasetID string, expectedVersion int64, now time.Time) error {
	result, err := q.ExecContext(ctx, `
		UPDATE dataset_drafts
		SET item_count = (SELECT COUNT(*) FROM dataset_items WHERE tenant_id = ? AND dataset_id = ?),
		    updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND status = 'draft' AND version = ?`,
		tenantID, datasetID, storage.FormatTime(now), tenantID, datasetID, expectedVersion)
	if err != nil {
		return fmt.Errorf("refresh dataset item count: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read dataset item count result: %w", err)
	}
	if changed != 1 {
		return domain.Conflict("dataset.add_items", "dataset_draft", datasetID, "dataset changed or is no longer editable")
	}
	return nil
}

func (Repository) Transition(ctx context.Context, q storage.Queryer, tenantID, datasetID string, from, to domain.DatasetStatus, expectedVersion int64, digest string, now time.Time) error {
	result, err := q.ExecContext(ctx, `
		UPDATE dataset_drafts
		SET status = ?, digest = CASE WHEN ? = '' THEN digest ELSE ? END,
		    frozen_at = CASE WHEN ? = 'frozen' THEN ? ELSE frozen_at END,
		    updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND status = ? AND version = ?`,
		to, digest, digest, to, storage.FormatTime(now), storage.FormatTime(now),
		tenantID, datasetID, from, expectedVersion)
	if err != nil {
		return fmt.Errorf("transition dataset draft: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read dataset transition result: %w", err)
	}
	if changed != 1 {
		return domain.Conflict("dataset.transition", "dataset_draft", datasetID, "state or version changed")
	}
	return nil
}

func (Repository) InsertReview(ctx context.Context, q storage.Queryer, review domain.QualityReview) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO quality_reviews(
			id, tenant_id, object_type, object_id, reviewer_id,
			outcome, reason, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, review.ID, review.TenantID,
		review.ObjectType, review.ObjectID, review.ReviewerID, review.Outcome,
		review.Reason, storage.FormatTime(review.CreatedAt), storage.FormatTime(review.UpdatedAt), review.Version)
	if err != nil {
		return fmt.Errorf("insert quality review: %w", err)
	}
	return nil
}

func (Repository) InsertRelease(ctx context.Context, q storage.Queryer, release domain.DatasetRelease, items []domain.DatasetItem) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO dataset_releases(
			id, tenant_id, dataset_id, revision, digest, status,
			published_at, revoked_at, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, release.ID, release.TenantID,
		release.DatasetID, release.Revision, release.Digest, release.Status,
		storage.NullableTime(release.PublishedAt), storage.NullableTime(release.RevokedAt),
		storage.FormatTime(release.CreatedAt), storage.FormatTime(release.UpdatedAt), release.Version)
	if err != nil {
		return fmt.Errorf("insert dataset release: %w", err)
	}
	for _, item := range items {
		if _, err := q.ExecContext(ctx, `
			INSERT INTO release_items(release_id, dataset_item_id, tenant_id, created_at)
			VALUES(?, ?, ?, ?)`, release.ID, item.ID, release.TenantID, storage.FormatTime(release.CreatedAt)); err != nil {
			return fmt.Errorf("insert release item %s: %w", item.ID, err)
		}
	}
	return nil
}

func (Repository) FindRelease(ctx context.Context, q storage.Queryer, tenantID, id string) (domain.DatasetRelease, error) {
	var value domain.DatasetRelease
	var published, revoked sql.NullString
	var created, updated string
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, dataset_id, revision, digest, status,
		       published_at, revoked_at, created_at, updated_at, version
		FROM dataset_releases WHERE tenant_id = ? AND id = ?`, tenantID, id,
	).Scan(&value.ID, &value.TenantID, &value.DatasetID, &value.Revision,
		&value.Digest, &value.Status, &published, &revoked, &created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DatasetRelease{}, domain.NotFound("dataset.find_release", "dataset_release", id)
	}
	if err != nil {
		return domain.DatasetRelease{}, fmt.Errorf("find dataset release: %w", err)
	}
	var parseErr error
	if value.PublishedAt, parseErr = storage.ScanNullableTime(published); parseErr != nil {
		return domain.DatasetRelease{}, fmt.Errorf("parse release published_at: %w", parseErr)
	}
	if value.RevokedAt, parseErr = storage.ScanNullableTime(revoked); parseErr != nil {
		return domain.DatasetRelease{}, fmt.Errorf("parse release revoked_at: %w", parseErr)
	}
	if value.CreatedAt, parseErr = storage.ParseTime(created); parseErr != nil {
		return domain.DatasetRelease{}, fmt.Errorf("parse release created_at: %w", parseErr)
	}
	if value.UpdatedAt, parseErr = storage.ParseTime(updated); parseErr != nil {
		return domain.DatasetRelease{}, fmt.Errorf("parse release updated_at: %w", parseErr)
	}
	return value, nil
}

func (Repository) CountActiveJobs(ctx context.Context, q storage.Queryer, tenantID, releaseID string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM training_jobs
		WHERE tenant_id = ? AND release_id = ? AND status IN ('queued','running','retrying')`, tenantID, releaseID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active release jobs: %w", err)
	}
	return count, nil
}

func (Repository) TransitionRelease(ctx context.Context, q storage.Queryer, tenantID, releaseID string, from, to domain.DatasetStatus, version int64, now time.Time) error {
	published, revoked := any(nil), any(nil)
	if to == domain.DatasetStatusPublished {
		published = storage.FormatTime(now)
	}
	if to == domain.DatasetStatusRevoked {
		revoked = storage.FormatTime(now)
	}
	result, err := q.ExecContext(ctx, `
		UPDATE dataset_releases
		SET status = ?, published_at = COALESCE(?, published_at),
		    revoked_at = COALESCE(?, revoked_at), updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND status = ? AND version = ?`,
		to, published, revoked, storage.FormatTime(now), tenantID, releaseID, from, version)
	if err != nil {
		return fmt.Errorf("transition dataset release: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read release transition result: %w", err)
	}
	if changed != 1 {
		return domain.Conflict("dataset.transition_release", "dataset_release", releaseID, "state or version changed")
	}
	return nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

type scanner interface {
	Scan(...any) error
}
