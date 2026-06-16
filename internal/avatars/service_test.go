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

// seedUserUnique seeds a user with a unique contact code derived from userID,
// allowing multiple distinct users to coexist in the same test DB.
func seedUserUnique(t *testing.T, d db.DB, userID string) {
	t.Helper()
	// Build a contact code from the first 8 characters of userID, padded if needed.
	code := userID
	for len(code) < 8 {
		code += "A"
	}
	code = code[:4] + "-" + code[4:8]
	// INSERT OR IGNORE makes this idempotent — safe to call multiple times for the
	// same userID (e.g. when the commit helper seeds before each RequestUpload call).
	_, _ = d.Run(context.Background(),
		`INSERT OR IGNORE INTO users (id, contact_code, display_name, public_key) VALUES (?, ?, ?, ?)`,
		userID, code, "User-"+userID, "pk-"+userID)
}

// commit is a test helper: request-upload, simulate the S3 PUT by setting the
// mock size, then call Commit. The user is seeded via seedUserUnique before
// inserting the avatar row.
func commit(t *testing.T, svc *Service, st *mockStore, d db.DB, user, kind string, size int64) string {
	t.Helper()
	seedUserUnique(t, d, user)
	id, _, err := svc.RequestUpload(context.Background(), user, kind, size)
	if err != nil {
		t.Fatal(err)
	}
	row, _ := d.Get(context.Background(), `SELECT object_key FROM avatars WHERE id = ?`, id)
	st.sizes[row.Str("object_key")] = size // simulate the S3 PUT
	if err := svc.Commit(context.Background(), id, user); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCommit_AndOwnerCanDownload(t *testing.T) {
	svc, d, st := newTestService(t)
	id := commit(t, svc, st, d, "owner", "default", 500)
	if _, err := svc.DownloadURL(context.Background(), id, "owner"); err != nil {
		t.Fatalf("owner download: %v", err)
	}
}

func TestCommit_SizeMismatch(t *testing.T) {
	svc, d, st := newTestService(t)
	seedUserUnique(t, d, "owner")
	avatarID, _, err := svc.RequestUpload(context.Background(), "owner", "default", 500)
	if err != nil {
		t.Fatal(err)
	}
	row, _ := d.Get(context.Background(), `SELECT object_key FROM avatars WHERE id = ?`, avatarID)
	st.sizes[row.Str("object_key")] = 999 // different from declared 500
	if err := svc.Commit(context.Background(), avatarID, "owner"); err != ErrSizeMismatch {
		t.Fatalf("want ErrSizeMismatch, got %v", err)
	}
}

func TestDownload_ForbiddenForStranger(t *testing.T) {
	svc, d, st := newTestService(t)
	id := commit(t, svc, st, d, "owner", "default", 500)
	if _, err := svc.DownloadURL(context.Background(), id, "stranger"); err != ErrForbidden {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestDownload_AllowedForAcceptedContact(t *testing.T) {
	svc, d, st := newTestService(t)
	id := commit(t, svc, st, d, "owner", "default", 500)
	// seed "friend" then insert the accepted contact edge owner -> friend
	seedUserUnique(t, d, "friend")
	_, _ = d.Run(context.Background(),
		`INSERT INTO contacts (user_id, contact_id, status) VALUES ('owner', 'friend', 'accepted')`)
	if _, err := svc.DownloadURL(context.Background(), id, "friend"); err != nil {
		t.Fatalf("contact download: %v", err)
	}
}

func TestDeleteCustom_RemovesRow(t *testing.T) {
	svc, d, st := newTestService(t)
	commit(t, svc, st, d, "owner", "custom", 500)
	if err := svc.DeleteCustom(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	row, _ := d.Get(context.Background(), `SELECT 1 FROM avatars WHERE user_id = 'owner' AND kind = 'custom'`)
	if row != nil {
		t.Fatal("custom avatar should be gone")
	}
}

func TestCommit_ReplacesOldCommitted(t *testing.T) {
	ctx := context.Background()
	svc, d, st := newTestService(t)

	// Commit avatar A for user "owner" (commit helper seeds the user via seedUserUnique).
	aID := commit(t, svc, st, d, "owner", "default", 500)

	// Capture A's object_key before B's commit deletes it from the store.
	aRow, _ := d.Get(ctx, `SELECT object_key FROM avatars WHERE id = ?`, aID)
	if aRow == nil {
		t.Fatal("expected row for avatar A before second commit")
	}
	aObjectKey := aRow.Str("object_key")

	// Commit avatar B — same user, same kind. seedUserUnique inside commit uses
	// INSERT OR IGNORE so seeding "owner" a second time is harmless.
	bID := commit(t, svc, st, d, "owner", "default", 500)

	// A's DB row must be gone.
	gone, _ := d.Get(ctx, `SELECT 1 FROM avatars WHERE id = ?`, aID)
	if gone != nil {
		t.Fatalf("expected avatar A row (%s) to be deleted after B was committed", aID)
	}

	// Exactly one committed 'default' row must remain for "owner".
	countRow, _ := d.Get(ctx,
		`SELECT COUNT(*) AS n FROM avatars WHERE user_id = 'owner' AND kind = 'default' AND status = 'committed'`)
	if countRow == nil || countRow.Int("n") != 1 {
		t.Fatalf("expected exactly 1 committed default row, got %v", countRow)
	}

	// B's row must be that one row.
	bRow, _ := d.Get(ctx, `SELECT status FROM avatars WHERE id = ?`, bID)
	if bRow == nil || bRow.Str("status") != "committed" {
		t.Fatalf("expected avatar B (%s) to be committed", bID)
	}

	// A's object_key must have been removed from the mock store.
	if _, ok := st.sizes[aObjectKey]; ok {
		t.Fatalf("expected A's object_key %q to be deleted from the store, but it still exists", aObjectKey)
	}
}

func TestRequestUpload_KeepsCommittedRow(t *testing.T) {
	ctx := context.Background()
	svc, d, st := newTestService(t)

	// Commit avatar A for user "owner".
	aID := commit(t, svc, st, d, "owner", "default", 500)

	// Verify A is committed.
	aRow, _ := d.Get(ctx, `SELECT status FROM avatars WHERE id = ?`, aID)
	if aRow == nil || aRow.Str("status") != "committed" {
		t.Fatalf("expected avatar A to be committed, got %v", aRow)
	}

	// Now request a new upload for the same (user, kind) — do NOT commit it.
	_, _, err := svc.RequestUpload(ctx, "owner", "default", 300)
	if err != nil {
		t.Fatalf("RequestUpload failed: %v", err)
	}

	// A's committed row must still exist.
	aRowAfter, _ := d.Get(ctx, `SELECT status FROM avatars WHERE id = ?`, aID)
	if aRowAfter == nil {
		t.Fatal("expected avatar A's committed row to survive RequestUpload, but it was deleted")
	}
	if aRowAfter.Str("status") != "committed" {
		t.Fatalf("expected avatar A to remain committed, got status=%q", aRowAfter.Str("status"))
	}

	// There must now be both a committed row (A) and a pending row — two rows total for (owner, default).
	countRow, _ := d.Get(ctx,
		`SELECT COUNT(*) AS n FROM avatars WHERE user_id = 'owner' AND kind = 'default'`)
	if countRow == nil || countRow.Int("n") != 2 {
		t.Fatalf("expected 2 rows (committed A + pending B), got %v", countRow)
	}

	// Exactly one of those must be pending.
	pendingRow, _ := d.Get(ctx,
		`SELECT COUNT(*) AS n FROM avatars WHERE user_id = 'owner' AND kind = 'default' AND status = 'pending'`)
	if pendingRow == nil || pendingRow.Int("n") != 1 {
		t.Fatalf("expected exactly 1 pending row, got %v", pendingRow)
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
