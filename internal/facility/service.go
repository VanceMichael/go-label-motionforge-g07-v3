package facility

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/audit"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/clock"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

type Service struct {
	db       *storage.Database
	repo     Repository
	audits   audit.Store
	clock    clock.Clock
	leaseTTL time.Duration
}

func NewService(db *storage.Database, c clock.Clock, leaseTTL time.Duration) *Service {
	return &Service{db: db, repo: Repository{}, audits: audit.Store{}, clock: c, leaseTTL: leaseTTL}
}

func (s *Service) CreateFacility(ctx context.Context, principal auth.Principal, name, timezone, requestID string) (domain.Facility, error) {
	if err := auth.RequireRole(principal, domain.RoleTenantAdmin); err != nil {
		return domain.Facility{}, err
	}
	name, timezone = strings.TrimSpace(name), strings.TrimSpace(timezone)
	if name == "" || timezone == "" || requestID == "" {
		return domain.Facility{}, domain.Validation("facility.create", "name, timezone, and request id are required")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return domain.Facility{}, domain.Validation("facility.create", "timezone is invalid")
	}
	id, err := domain.NewID("facility")
	if err != nil {
		return domain.Facility{}, err
	}
	auditID, err := domain.NewID("audit")
	if err != nil {
		return domain.Facility{}, err
	}
	now := s.clock.Now()
	value := domain.Facility{ID: id, TenantID: principal.TenantID, Name: name, Timezone: timezone, Active: true, CreatedAt: now, UpdatedAt: now, Version: 1}
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		if err := s.repo.InsertFacility(ctx, tx, value); err != nil {
			return err
		}
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "facility.create", ObjectType: "facility", ObjectID: value.ID, Outcome: "created", RequestID: requestID, CreatedAt: now})
	})
	if err != nil {
		return domain.Facility{}, fmt.Errorf("create facility: %w", err)
	}
	return value, nil
}

func (s *Service) CreateRig(ctx context.Context, principal auth.Principal, facilityID, name string, capabilities domain.RigCapability, requestID string) (domain.CaptureRig, error) {
	if err := auth.RequireRole(principal, domain.RoleTenantAdmin, domain.RoleOperator); err != nil {
		return domain.CaptureRig{}, err
	}
	name = strings.TrimSpace(name)
	if facilityID == "" || name == "" || capabilities == 0 || requestID == "" {
		return domain.CaptureRig{}, domain.Validation("facility.create_rig", "facility, name, capabilities, and request id are required")
	}
	facility, err := s.repo.FindFacility(ctx, s.db.SQL(), principal.TenantID, facilityID)
	if err != nil {
		return domain.CaptureRig{}, err
	}
	if !facility.Active {
		return domain.CaptureRig{}, domain.Precondition("facility.create_rig", "facility", facilityID, "facility is inactive")
	}
	id, err := domain.NewID("rig")
	if err != nil {
		return domain.CaptureRig{}, err
	}
	auditID, err := domain.NewID("audit")
	if err != nil {
		return domain.CaptureRig{}, err
	}
	now := s.clock.Now()
	value := domain.CaptureRig{ID: id, TenantID: principal.TenantID, FacilityID: facilityID, Name: name, Capabilities: capabilities, Active: true, CreatedAt: now, UpdatedAt: now, Version: 1}
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		if err := s.repo.InsertRig(ctx, tx, value); err != nil {
			return err
		}
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "rig.create", ObjectType: "capture_rig", ObjectID: value.ID, Outcome: "created", RequestID: requestID, CreatedAt: now})
	})
	if err != nil {
		return domain.CaptureRig{}, fmt.Errorf("create rig: %w", err)
	}
	return value, nil
}

func (s *Service) CreateScenario(ctx context.Context, principal auth.Principal, name, environment string, required domain.RigCapability, requestID string) (domain.Scenario, error) {
	if err := auth.RequireRole(principal, domain.RoleTenantAdmin, domain.RoleDataSteward); err != nil {
		return domain.Scenario{}, err
	}
	name, environment = strings.TrimSpace(name), strings.TrimSpace(environment)
	if name == "" || environment == "" || required == 0 || requestID == "" {
		return domain.Scenario{}, domain.Validation("facility.create_scenario", "name, environment, capabilities, and request id are required")
	}
	id, err := domain.NewID("scenario")
	if err != nil {
		return domain.Scenario{}, err
	}
	auditID, err := domain.NewID("audit")
	if err != nil {
		return domain.Scenario{}, err
	}
	now := s.clock.Now()
	value := domain.Scenario{ID: id, TenantID: principal.TenantID, Name: name, Environment: environment, RequiredCapabilities: required, Active: true, CreatedAt: now, UpdatedAt: now, Version: 1}
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		if err := s.repo.InsertScenario(ctx, tx, value); err != nil {
			return err
		}
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "scenario.create", ObjectType: "scenario", ObjectID: value.ID, Outcome: "created", RequestID: requestID, CreatedAt: now})
	})
	if err != nil {
		return domain.Scenario{}, fmt.Errorf("create scenario: %w", err)
	}
	return value, nil
}

func (s *Service) ReserveRig(ctx context.Context, principal auth.Principal, rigID, captureID, requestID string) (domain.RigLease, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return domain.RigLease{}, err
	}
	if rigID == "" || captureID == "" || requestID == "" {
		return domain.RigLease{}, domain.Validation("facility.reserve_rig", "rig, capture, and request id are required")
	}
	rig, err := s.repo.FindRig(ctx, s.db.SQL(), principal.TenantID, rigID)
	if err != nil {
		return domain.RigLease{}, err
	}
	if !rig.Active {
		return domain.RigLease{}, domain.Precondition("facility.reserve_rig", "capture_rig", rigID, "rig is inactive")
	}
	leaseID, err := domain.NewID("lease")
	if err != nil {
		return domain.RigLease{}, err
	}
	token, err := leaseToken()
	if err != nil {
		return domain.RigLease{}, err
	}
	auditID, err := domain.NewID("audit")
	if err != nil {
		return domain.RigLease{}, err
	}
	now := s.clock.Now()
	lease := domain.RigLease{ID: leaseID, TenantID: principal.TenantID, RigID: rigID, CaptureID: captureID, Owner: principal.UserID, Token: token, ExpiresAt: now.Add(s.leaseTTL), CreatedAt: now, UpdatedAt: now, Version: 1}
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		if err := s.repo.InsertLease(ctx, tx, lease); err != nil {
			return domain.Conflict("facility.reserve_rig", "capture_rig", rigID, "rig already has a live lease")
		}
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "rig.reserve", ObjectType: "rig_lease", ObjectID: lease.ID, Outcome: "reserved", RequestID: requestID, CreatedAt: now})
	})
	if err != nil {
		return domain.RigLease{}, fmt.Errorf("reserve rig: %w", err)
	}
	return lease, nil
}

func (s *Service) RenewRig(ctx context.Context, principal auth.Principal, leaseID, rigID, token string, version int64, requestID string) (domain.RigLease, error) {
	if err := auth.RequireRole(principal, domain.RoleOperator); err != nil {
		return domain.RigLease{}, err
	}
	lease, err := s.repo.FindActiveLease(ctx, s.db.SQL(), principal.TenantID, rigID)
	if err != nil {
		return domain.RigLease{}, err
	}
	if lease.ID != leaseID || lease.Owner != principal.UserID || lease.Token != token || lease.Version != version {
		return domain.RigLease{}, domain.Wrap(domain.ErrLeaseLost, "facility.renew_rig", "rig_lease", leaseID, "lease identity or ownership changed", nil)
	}
	auditID, err := domain.NewID("audit")
	if err != nil {
		return domain.RigLease{}, err
	}
	now := s.clock.Now()
	expires := now.Add(s.leaseTTL)
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		if err := s.repo.RenewLease(ctx, tx, principal.TenantID, lease.ID, principal.UserID, token, version, now, expires); err != nil {
			return err
		}
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "rig.renew", ObjectType: "rig_lease", ObjectID: lease.ID, Outcome: "renewed", RequestID: requestID, CreatedAt: now})
	})
	if err != nil {
		return domain.RigLease{}, err
	}
	lease.ExpiresAt, lease.UpdatedAt, lease.Version = expires, now, lease.Version+1
	return lease, nil
}

func (s *Service) ReleaseRig(ctx context.Context, principal auth.Principal, rigID, leaseID, token string, version int64, requestID string) error {
	if err := auth.RequireRole(principal, domain.RoleOperator, domain.RoleDataSteward); err != nil {
		return err
	}
	lease, err := s.repo.FindActiveLease(ctx, s.db.SQL(), principal.TenantID, rigID)
	if err != nil {
		return err
	}
	auditID, err := domain.NewID("audit")
	if err != nil {
		return err
	}
	now := s.clock.Now()
	return s.db.Write(ctx, func(tx *sql.Tx) error {
		if err := s.repo.ReleaseLease(ctx, tx, principal.TenantID, lease.ID, principal.UserID, token, version, now); err != nil {
			return err
		}
		return s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: principal.TenantID, ActorID: principal.UserID, Action: "rig.release", ObjectType: "rig_lease", ObjectID: lease.ID, Outcome: "released", RequestID: requestID, CreatedAt: now})
	})
}

func leaseToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate lease token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
