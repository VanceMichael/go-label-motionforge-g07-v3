package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

func TestTask0004ConcurrentRigReservationHasOneOwner(t *testing.T) {
	e := newEnvironment(t)
	second := e.createPrincipal(t, "operator-two@motion.test", "Second Operator", domain.RoleOperator)
	facilityValue, err := e.facilities.CreateFacility(context.Background(), e.admin, "Task Four Lab", "Asia/Shanghai", "task0004-facility")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := e.facilities.CreateRig(context.Background(), e.admin, facilityValue.ID, "Shared Rig", domain.CapabilityPose, "task0004-rig")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := e.facilities.CreateScenario(context.Background(), e.admin, "Concurrent capture", "lab", domain.CapabilityPose, "task0004-scenario")
	if err != nil {
		t.Fatal(err)
	}
	now := storage.FormatTime(e.clock.Now())
	for _, id := range []string{"capture-task0004-a", "capture-task0004-b"} {
		if _, err := e.database.SQL().ExecContext(context.Background(), `
			INSERT INTO capture_sessions(
				id, tenant_id, facility_id, scenario_id, rig_id, operator_id,
				status, revision, consent_ref, created_at, updated_at, version
			) VALUES(?, ?, ?, ?, ?, ?, 'planned', 1, ?, ?, ?, 1)`,
			id, e.admin.TenantID, facilityValue.ID, scenario.ID, rig.ID,
			e.operator.UserID, "consent-"+id, now, now); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, err := e.facilities.ReserveRig(context.Background(), e.operator, rig.ID, "capture-task0004-a", "task0004-a")
		errs <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := e.facilities.ReserveRig(context.Background(), second, rig.ID, "capture-task0004-b", "task0004-b")
		errs <- err
	}()
	close(start)
	wait.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent reservation produced %d successful owners: %v", successes, fmt.Sprint(errs))
	}
	var leases int
	if err := e.database.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM rig_leases WHERE tenant_id = ? AND rig_id = ? AND released_at IS NULL`, e.admin.TenantID, rig.ID).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if leases != 1 {
		t.Fatalf("expected one live rig lease, found %d", leases)
	}
}
