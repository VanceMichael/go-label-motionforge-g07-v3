package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
)

func TestTask0003LogoutAuditFailurePreservesSession(t *testing.T) {
	e := newEnvironment(t)
	login, err := e.auth.Login(context.Background(), auth.LoginInput{
		TenantID:  e.admin.TenantID,
		Email:     "admin@motion.test",
		Password:  "test-password-admin",
		RequestID: "task0003-login",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER task0003_fail_audit BEFORE INSERT ON audit_events
		BEGIN SELECT RAISE(ABORT, 'audit sink unavailable'); END`); err != nil {
		t.Fatal(err)
	}

	principal := auth.Principal{TenantID: login.TenantID, UserID: login.UserID, Role: login.Role, SessionID: login.SessionID}
	if err := e.auth.Logout(context.Background(), principal, "task0003-logout"); err == nil {
		t.Fatal("logout succeeded despite rejected audit event")
	}
	if _, err := e.auth.Authenticate(context.Background(), login.Token); err != nil {
		t.Fatalf("session was revoked even though logout failed: %v", err)
	}
}
