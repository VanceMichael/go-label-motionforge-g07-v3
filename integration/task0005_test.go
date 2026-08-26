package integration_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

func TestTask0005StaleRigReleaseCannotFreeCurrentOwner(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	oldLease := fixture.plan.Lease
	second := e.createPrincipal(t, "operator-five@motion.test", "Replacement Operator", domain.RoleOperator)
	secondCapture := "capture-task0005-replacement"
	now := storage.FormatTime(e.clock.Now())
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		INSERT INTO capture_sessions(
			id, tenant_id, facility_id, scenario_id, rig_id, operator_id,
			status, revision, consent_ref, created_at, updated_at, version
		) VALUES(?, ?, ?, ?, ?, ?, 'planned', 1, ?, ?, ?, 1)`,
		secondCapture, e.admin.TenantID, fixture.facility.ID, fixture.scenario.ID, fixture.rig.ID,
		second.UserID, "consent-task0005", now, now); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(3 * time.Minute)
	if _, err := e.recovery.RecoverExpired(context.Background(), e.admin.TenantID, e.admin.UserID, "task0005-recovery"); err != nil {
		t.Fatal(err)
	}
	newLease, err := e.facilities.ReserveRig(context.Background(), second, fixture.rig.ID, secondCapture, "task0005-reclaim")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.facilities.ReleaseRig(context.Background(), e.operator, fixture.rig.ID, oldLease.ID, oldLease.Token, oldLease.Version, "task0005-stale-release"); err == nil {
		t.Fatal("stale owner released a rig after it was reclaimed")
	}
	var released sql.NullString
	if err := e.database.SQL().QueryRowContext(context.Background(),
		`SELECT released_at FROM rig_leases WHERE id = ?`, newLease.ID).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if released.Valid {
		t.Fatalf("current owner's lease was released by stale token at %s", released.String)
	}
}
