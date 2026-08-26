package facility

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

func (Repository) InsertFacility(ctx context.Context, q storage.Queryer, value domain.Facility) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO facilities(id, tenant_id, name, timezone, active, created_at, updated_at, version)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.TenantID, value.Name,
		value.Timezone, value.Active, storage.FormatTime(value.CreatedAt), storage.FormatTime(value.UpdatedAt), value.Version)
	if err != nil {
		return fmt.Errorf("insert facility: %w", err)
	}
	return nil
}

func (Repository) InsertRig(ctx context.Context, q storage.Queryer, value domain.CaptureRig) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO capture_rigs(
			id, tenant_id, facility_id, name, capabilities, active, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.TenantID, value.FacilityID,
		value.Name, value.Capabilities, value.Active, storage.FormatTime(value.CreatedAt),
		storage.FormatTime(value.UpdatedAt), value.Version)
	if err != nil {
		return fmt.Errorf("insert capture rig: %w", err)
	}
	return nil
}

func (Repository) InsertScenario(ctx context.Context, q storage.Queryer, value domain.Scenario) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO scenarios(
			id, tenant_id, name, environment, required_capabilities,
			active, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.TenantID, value.Name,
		value.Environment, value.RequiredCapabilities, value.Active,
		storage.FormatTime(value.CreatedAt), storage.FormatTime(value.UpdatedAt), value.Version)
	if err != nil {
		return fmt.Errorf("insert scenario: %w", err)
	}
	return nil
}

func (Repository) FindFacility(ctx context.Context, q storage.Queryer, tenantID, id string) (domain.Facility, error) {
	var value domain.Facility
	var created, updated string
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, timezone, active, created_at, updated_at, version
		FROM facilities WHERE tenant_id = ? AND id = ?`, tenantID, id,
	).Scan(&value.ID, &value.TenantID, &value.Name, &value.Timezone, &value.Active, &created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Facility{}, domain.NotFound("facility.find", "facility", id)
	}
	if err != nil {
		return domain.Facility{}, fmt.Errorf("find facility: %w", err)
	}
	value.CreatedAt, err = storage.ParseTime(created)
	if err != nil {
		return domain.Facility{}, fmt.Errorf("parse facility created_at: %w", err)
	}
	value.UpdatedAt, err = storage.ParseTime(updated)
	if err != nil {
		return domain.Facility{}, fmt.Errorf("parse facility updated_at: %w", err)
	}
	return value, nil
}

func (Repository) FindRig(ctx context.Context, q storage.Queryer, tenantID, id string) (domain.CaptureRig, error) {
	var value domain.CaptureRig
	var created, updated string
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, facility_id, name, capabilities, active, created_at, updated_at, version
		FROM capture_rigs WHERE tenant_id = ? AND id = ?`, tenantID, id,
	).Scan(&value.ID, &value.TenantID, &value.FacilityID, &value.Name,
		&value.Capabilities, &value.Active, &created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CaptureRig{}, domain.NotFound("facility.find_rig", "capture_rig", id)
	}
	if err != nil {
		return domain.CaptureRig{}, fmt.Errorf("find capture rig: %w", err)
	}
	value.CreatedAt, err = storage.ParseTime(created)
	if err != nil {
		return domain.CaptureRig{}, fmt.Errorf("parse rig created_at: %w", err)
	}
	value.UpdatedAt, err = storage.ParseTime(updated)
	if err != nil {
		return domain.CaptureRig{}, fmt.Errorf("parse rig updated_at: %w", err)
	}
	return value, nil
}

func (Repository) FindScenario(ctx context.Context, q storage.Queryer, tenantID, id string) (domain.Scenario, error) {
	var value domain.Scenario
	var created, updated string
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, environment, required_capabilities,
		       active, created_at, updated_at, version
		FROM scenarios WHERE tenant_id = ? AND id = ?`, tenantID, id,
	).Scan(&value.ID, &value.TenantID, &value.Name, &value.Environment,
		&value.RequiredCapabilities, &value.Active, &created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Scenario{}, domain.NotFound("facility.find_scenario", "scenario", id)
	}
	if err != nil {
		return domain.Scenario{}, fmt.Errorf("find scenario: %w", err)
	}
	value.CreatedAt, err = storage.ParseTime(created)
	if err != nil {
		return domain.Scenario{}, fmt.Errorf("parse scenario created_at: %w", err)
	}
	value.UpdatedAt, err = storage.ParseTime(updated)
	if err != nil {
		return domain.Scenario{}, fmt.Errorf("parse scenario updated_at: %w", err)
	}
	return value, nil
}

func (Repository) InsertLease(ctx context.Context, q storage.Queryer, value domain.RigLease) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO rig_leases(
			id, tenant_id, rig_id, capture_id, owner, token,
			expires_at, released_at, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.TenantID,
		value.RigID, value.CaptureID, value.Owner, value.Token,
		storage.FormatTime(value.ExpiresAt), storage.NullableTime(value.ReleasedAt),
		storage.FormatTime(value.CreatedAt), storage.FormatTime(value.UpdatedAt), value.Version)
	if err != nil {
		return fmt.Errorf("insert rig lease: %w", err)
	}
	return nil
}

func (Repository) FindActiveLease(ctx context.Context, q storage.Queryer, tenantID, rigID string) (domain.RigLease, error) {
	var value domain.RigLease
	var expires, created, updated string
	var released sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT id, tenant_id, rig_id, capture_id, owner, token,
		       expires_at, released_at, created_at, updated_at, version
		FROM rig_leases
		WHERE tenant_id = ? AND rig_id = ? AND released_at IS NULL`, tenantID, rigID,
	).Scan(&value.ID, &value.TenantID, &value.RigID, &value.CaptureID,
		&value.Owner, &value.Token, &expires, &released, &created, &updated, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RigLease{}, domain.NotFound("facility.find_active_lease", "rig_lease", rigID)
	}
	if err != nil {
		return domain.RigLease{}, fmt.Errorf("find active rig lease: %w", err)
	}
	var parseErr error
	if value.ExpiresAt, parseErr = storage.ParseTime(expires); parseErr != nil {
		return domain.RigLease{}, fmt.Errorf("parse lease expires_at: %w", parseErr)
	}
	if value.ReleasedAt, parseErr = storage.ScanNullableTime(released); parseErr != nil {
		return domain.RigLease{}, fmt.Errorf("parse lease released_at: %w", parseErr)
	}
	if value.CreatedAt, parseErr = storage.ParseTime(created); parseErr != nil {
		return domain.RigLease{}, fmt.Errorf("parse lease created_at: %w", parseErr)
	}
	if value.UpdatedAt, parseErr = storage.ParseTime(updated); parseErr != nil {
		return domain.RigLease{}, fmt.Errorf("parse lease updated_at: %w", parseErr)
	}
	return value, nil
}

func (Repository) RenewLease(ctx context.Context, q storage.Queryer, tenantID, leaseID, owner, token string, version int64, now, expiresAt time.Time) error {
	result, err := q.ExecContext(ctx, `
		UPDATE rig_leases
		SET expires_at = ?, updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND owner = ? AND token = ?
		  AND version = ? AND released_at IS NULL AND expires_at > ?`,
		storage.FormatTime(expiresAt), storage.FormatTime(now), tenantID, leaseID,
		owner, token, version, storage.FormatTime(now))
	if err != nil {
		return fmt.Errorf("renew rig lease: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rig renewal result: %w", err)
	}
	if changed != 1 {
		return domain.Wrap(domain.ErrLeaseLost, "facility.renew_lease", "rig_lease", leaseID, "lease expired, changed, or belongs to another owner", nil)
	}
	return nil
}

func (Repository) ReleaseLease(ctx context.Context, q storage.Queryer, tenantID, leaseID, owner, token string, version int64, now time.Time) error {
	result, err := q.ExecContext(ctx, `
		UPDATE rig_leases
		SET released_at = ?, updated_at = ?, version = version + 1
		WHERE tenant_id = ? AND id = ? AND owner = ? AND token = ?
		  AND version = ? AND released_at IS NULL`, storage.FormatTime(now),
		storage.FormatTime(now), tenantID, leaseID, owner, token, version)
	if err != nil {
		return fmt.Errorf("release rig lease: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rig release result: %w", err)
	}
	if changed != 1 {
		return domain.Wrap(domain.ErrLeaseLost, "facility.release_lease", "rig_lease", leaseID, "lease expired, changed, or belongs to another owner", nil)
	}
	return nil
}
