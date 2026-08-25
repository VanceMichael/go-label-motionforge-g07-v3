package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
)

func TestTask0001BootstrapAuditFailureRollsBackTenant(t *testing.T) {
	e := newEnvironment(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER task0001_fail_audit BEFORE INSERT ON audit_events
		BEGIN SELECT RAISE(ABORT, 'audit sink unavailable'); END`); err != nil {
		t.Fatal(err)
	}

	_, _, err := e.auth.Bootstrap(context.Background(), auth.BootstrapInput{
		TenantName:  "Embodied Data Lab",
		Email:       "bootstrap@task0001.motion",
		DisplayName: "Bootstrap Admin",
		Password:    "task-password",
	})
	if err == nil {
		t.Fatal("bootstrap succeeded while its audit record was rejected")
	}
	var tenants, users int
	if err := e.database.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM tenants WHERE name = 'Embodied Data Lab'`).Scan(&tenants); err != nil {
		t.Fatal(err)
	}
	if err := e.database.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM users WHERE email = 'bootstrap@task0001.motion'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if tenants != 0 || users != 0 {
		t.Fatalf("failed bootstrap left durable identity rows: tenants=%d users=%d", tenants, users)
	}
}
