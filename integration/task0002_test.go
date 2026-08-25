package integration_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
)

func TestTask0002CanceledLoginLeavesNoSession(t *testing.T) {
	e := newEnvironment(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.auth.Login(ctx, auth.LoginInput{
		TenantID:  e.admin.TenantID,
		Email:     "admin@motion.test",
		Password:  "test-password-admin",
		RequestID: "task0002-login",
	})
	if err == nil {
		t.Fatal("canceled login succeeded")
	}
	var sessions int
	if err := e.database.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM auth_sessions WHERE tenant_id = ?`, e.admin.TenantID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("canceled login persisted %d session(s)", sessions)
	}
}
