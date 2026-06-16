package avatars

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

// mockStore implements storage.Storage for tests.
type mockStore struct{ sizes map[string]int64 }

func newMockStore() *mockStore { return &mockStore{sizes: map[string]int64{}} }

func (m *mockStore) PresignPut(_ context.Context, key string, _ time.Duration) (string, error) {
	return "put://" + key, nil
}
func (m *mockStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "get://" + key, nil
}
func (m *mockStore) Size(_ context.Context, key string) (int64, error) {
	return m.sizes[key], nil
}
func (m *mockStore) Delete(_ context.Context, key string) error {
	delete(m.sizes, key)
	return nil
}

func newTestService(t *testing.T) (*Service, db.DB, *mockStore) {
	t.Helper()
	d, err := db.Open("sqlite", filepath.Join(t.TempDir(), "avatars.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InitSchema(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := newMockStore()
	return New(d, st, 2*1024*1024), d, st
}

func TestRequestUpload_RejectsBadKind(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, _, err := svc.RequestUpload(context.Background(), "u1", "banner", 100); err != ErrBadKind {
		t.Fatalf("want ErrBadKind, got %v", err)
	}
}

func TestRequestUpload_RejectsTooLarge(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, _, err := svc.RequestUpload(context.Background(), "u1", "default", 99*1024*1024); err != ErrTooLarge {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

func seedUser(t *testing.T, d db.DB, userID string) {
	t.Helper()
	_, _ = d.Run(context.Background(),
		`INSERT INTO users (id, contact_code, display_name, public_key) VALUES (?, ?, ?, ?)`,
		userID, "AAAA-2222", "Alice", "pk-"+userID)
}

func TestRequestUpload_InsertsPendingRow(t *testing.T) {
	svc, d, _ := newTestService(t)
	seedUser(t, d, "u1")
	id, url, err := svc.RequestUpload(context.Background(), "u1", "default", 1000)
	if err != nil || id == "" || url == "" {
		t.Fatalf("unexpected: id=%q url=%q err=%v", id, url, err)
	}
	row, _ := d.Get(context.Background(), `SELECT status FROM avatars WHERE id = ?`, id)
	if row == nil || row.Str("status") != "pending" {
		t.Fatalf("expected pending row, got %v", row)
	}
}

func TestRequestUpload_ReplacesExistingPendingRow(t *testing.T) {
	ctx := context.Background()
	svc, d, _ := newTestService(t)
	seedUser(t, d, "u1")

	// First upload: capture the avatarID and the object_key it created.
	firstID, _, err := svc.RequestUpload(ctx, "u1", "default", 1000)
	if err != nil {
		t.Fatalf("first RequestUpload failed: %v", err)
	}
	firstRow, _ := d.Get(ctx, `SELECT object_key FROM avatars WHERE id = ?`, firstID)
	if firstRow == nil {
		t.Fatal("expected row for first avatarID, got nil")
	}

	// Second upload for the same (user, kind).
	secondID, _, err := svc.RequestUpload(ctx, "u1", "default", 1000)
	if err != nil {
		t.Fatalf("second RequestUpload failed: %v", err)
	}

	// The two IDs must differ.
	if firstID == secondID {
		t.Fatalf("expected distinct avatarIDs, both are %q", firstID)
	}

	// Exactly one pending row for (user_id, kind) must survive.
	countRow, _ := d.Get(ctx,
		`SELECT COUNT(*) AS n FROM avatars WHERE user_id = ? AND kind = 'default'`, "u1")
	if countRow == nil || countRow.Int("n") != 1 {
		t.Fatalf("expected 1 pending row, got %v", countRow)
	}

	// The old DB row (first avatarID) must be gone.
	gone, _ := d.Get(ctx, `SELECT 1 FROM avatars WHERE id = ?`, firstID)
	if gone != nil {
		t.Fatalf("old avatarID %q should have been deleted but still exists", firstID)
	}
}
