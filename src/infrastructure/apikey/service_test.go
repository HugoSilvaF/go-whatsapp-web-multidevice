package apikey

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "apikey_test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	svc := NewService(db)
	if err := svc.InitializeSchema(); err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") || strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("skipping apikey sqlite integration tests without cgo: %v", err)
		}
		t.Fatalf("failed to initialize schema: %v", err)
	}
	return svc, db
}

func TestService_CreateAndAuthenticate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	meta, key, err := svc.CreateKey(ctx, CreateParams{
		Name:   "bot",
		Scopes: []string{"messages:send", "chats:read"},
	})
	if err != nil {
		t.Fatalf("create key failed: %v", err)
	}
	if meta.ID == "" || key == "" {
		t.Fatalf("expected metadata id and key to be set")
	}

	auth, err := svc.Authenticate(ctx, key)
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if auth.KeyID != meta.ID {
		t.Fatalf("expected key id %s got %s", meta.ID, auth.KeyID)
	}
	if len(auth.Scopes) != 2 {
		t.Fatalf("expected 2 scopes got %d", len(auth.Scopes))
	}
}

func TestService_RotateRevokesOldSecret(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	meta, key1, err := svc.CreateKey(ctx, CreateParams{
		Name:   "rotator",
		Scopes: []string{"messages:send"},
	})
	if err != nil {
		t.Fatalf("create key failed: %v", err)
	}

	_, key2, err := svc.RotateKey(ctx, meta.ID)
	if err != nil {
		t.Fatalf("rotate key failed: %v", err)
	}
	if key1 == key2 {
		t.Fatalf("expected rotated key to differ")
	}

	if _, err := svc.Authenticate(ctx, key1); err == nil {
		t.Fatalf("expected old key to be invalid after rotation")
	}

	if _, err := svc.Authenticate(ctx, key2); err != nil {
		t.Fatalf("expected new rotated key to authenticate: %v", err)
	}
}

func TestService_RevokeAndExpire(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	expired := time.Now().UTC().Add(-time.Hour)
	metaExpired, keyExpired, err := svc.CreateKey(ctx, CreateParams{
		Name:      "expired",
		Scopes:    []string{"chats:read"},
		ExpiresAt: &expired,
	})
	if err != nil {
		t.Fatalf("create expired key failed: %v", err)
	}
	if _, err := svc.Authenticate(ctx, keyExpired); err == nil {
		t.Fatalf("expected expired key auth to fail")
	}

	metaRev, keyRev, err := svc.CreateKey(ctx, CreateParams{
		Name:   "revoked",
		Scopes: []string{"messages:manage"},
	})
	if err != nil {
		t.Fatalf("create key failed: %v", err)
	}
	if err := svc.RevokeKey(ctx, metaRev.ID); err != nil {
		t.Fatalf("revoke key failed: %v", err)
	}
	if _, err := svc.Authenticate(ctx, keyRev); err == nil {
		t.Fatalf("expected revoked key auth to fail")
	}

	keys, err := svc.ListKeys(ctx, true)
	if err != nil {
		t.Fatalf("list keys failed: %v", err)
	}
	if len(keys) < 2 {
		t.Fatalf("expected at least 2 keys, got %d", len(keys))
	}
	if metaExpired.ID == "" || metaRev.ID == "" {
		t.Fatalf("expected valid ids")
	}
}
