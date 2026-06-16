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
