package files

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

// mockStorage records calls and returns canned values.
type mockStorage struct {
	putURL  string
	getURL  string
	size    int64
	sizeErr error
	deleted []string
}

func (m *mockStorage) PresignPut(_ context.Context, key string, _ time.Duration) (string, error) {
	return m.putURL + "/" + key, nil
}
func (m *mockStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return m.getURL + "/" + key, nil
}
func (m *mockStorage) Size(_ context.Context, _ string) (int64, error) { return m.size, m.sizeErr }
func (m *mockStorage) Delete(_ context.Context, key string) error {
	m.deleted = append(m.deleted, key)
	return nil
}

func newDB(t *testing.T) db.DB {
	t.Helper()
	d, err := db.Open("sqlite", filepath.Join(t.TempDir(), "files.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InitSchema(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func seedUsers(t *testing.T, d db.DB) {
	t.Helper()
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uA','AAAA-2222','A','pkA')`)
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uB','BBBB-3333','B','pkB')`)
	_, _ = d.Run(ctx, `INSERT INTO contacts (user_id, contact_id, status) VALUES ('uA','uB','accepted')`)
	_, _ = d.Run(ctx, `INSERT INTO contacts (user_id, contact_id, status) VALUES ('uB','uA','accepted')`)
}

func TestRequestUpload_OK(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{putURL: "https://put"}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)

	fileID, url, err := svc.RequestUpload(context.Background(), "uA", "uB", 1000)
	if err != nil {
		t.Fatalf("RequestUpload: %v", err)
	}
	if fileID == "" || url == "" {
		t.Fatalf("empty fileID/url: %q %q", fileID, url)
	}
	row, _ := d.Get(context.Background(), `SELECT status, size_bytes, sender_id, receiver_id FROM files WHERE id = ?`, fileID)
	if row == nil || row.Str("status") != "pending" || row.Int("size_bytes") != 1000 {
		t.Fatalf("pending row not written correctly: %+v", row)
	}
}

func TestRequestUpload_TooLarge(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	svc := New(d, &mockStorage{}, 25*1024*1024, 10*1024*1024*1024)
	_, _, err := svc.RequestUpload(context.Background(), "uA", "uB", 26*1024*1024)
	if err != ErrTooLarge {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestRequestUpload_StorageFull(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, created_at) VALUES ('f0','uA','uB','k0',900,'committed',1)`)
	svc := New(d, &mockStorage{}, 25*1024*1024, 1000)
	_, _, err := svc.RequestUpload(ctx, "uA", "uB", 200)
	if err != ErrStorageFull {
		t.Fatalf("err = %v, want ErrStorageFull", err)
	}
}

func TestRequestUpload_PerUserQuotaIsolated(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	ctx := context.Background()
	// uA has filled almost all of a 1000-byte PER-USER quota.
	_, _ = d.Run(ctx, `INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, created_at) VALUES ('f0','uA','uB','k0',900,'committed',1)`)
	svc := New(d, &mockStorage{putURL: "https://put"}, 25*1024*1024, 1000)
	// uA is over their own quota for a 200-byte upload...
	if _, _, err := svc.RequestUpload(ctx, "uA", "uB", 200); err != ErrStorageFull {
		t.Fatalf("uA err = %v, want ErrStorageFull", err)
	}
	// ...but uB's quota is independent and untouched.
	if _, _, err := svc.RequestUpload(ctx, "uB", "uA", 200); err != nil {
		t.Fatalf("uB should be allowed (independent quota): %v", err)
	}
}

func TestCommit_OK(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{putURL: "https://put", size: 1000}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	ctx := context.Background()

	fileID, _, _ := svc.RequestUpload(ctx, "uA", "uB", 1000)
	if err := svc.Commit(ctx, fileID, "uA", "msg-1"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	row, _ := d.Get(ctx, `SELECT status, message_id FROM files WHERE id = ?`, fileID)
	if row.Str("status") != "committed" || row.Str("message_id") != "msg-1" {
		t.Fatalf("not committed/linked: %+v", row)
	}
}

func TestCommit_WrongOwnerRejected(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{size: 1000}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	ctx := context.Background()
	fileID, _, _ := svc.RequestUpload(ctx, "uA", "uB", 1000)
	if err := svc.Commit(ctx, fileID, "uB", "msg-1"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCommit_SizeMismatchRejected(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{size: 999} // uploaded fewer bytes than declared
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	ctx := context.Background()
	fileID, _, _ := svc.RequestUpload(ctx, "uA", "uB", 1000)
	if err := svc.Commit(ctx, fileID, "uA", "msg-1"); err != ErrSizeMismatch {
		t.Fatalf("err = %v, want ErrSizeMismatch", err)
	}
}

func commitOne(t *testing.T, svc *Service, d db.DB, sender, receiver string) string {
	t.Helper()
	ctx := context.Background()
	fileID, _, err := svc.RequestUpload(ctx, sender, receiver, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Commit(ctx, fileID, sender, "msg-x"); err != nil {
		t.Fatal(err)
	}
	return fileID
}

func TestDownloadURL_SenderAndReceiverAllowed(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{getURL: "https://get", size: 1000}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	fileID := commitOne(t, svc, d, "uA", "uB")

	for _, who := range []string{"uA", "uB"} {
		url, err := svc.DownloadURL(context.Background(), fileID, who)
		if err != nil || url == "" {
			t.Fatalf("DownloadURL(%s) err=%v url=%q", who, err, url)
		}
	}
}

func TestDownloadURL_StrangerForbidden(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	_, _ = d.Run(context.Background(), `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uC','CCCC-4444','C','pkC')`)
	st := &mockStorage{getURL: "https://get", size: 1000}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	fileID := commitOne(t, svc, d, "uA", "uB")

	if _, err := svc.DownloadURL(context.Background(), fileID, "uC"); err != ErrForbidden {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestDownloadURL_PendingNotFound(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{getURL: "https://get"}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	ctx := context.Background()
	fileID, _, _ := svc.RequestUpload(ctx, "uA", "uB", 1000) // never committed
	if _, err := svc.DownloadURL(ctx, fileID, "uA"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteByMessage_RemovesObjectAndRow(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{size: 1000}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	ctx := context.Background()

	fileID, _, _ := svc.RequestUpload(ctx, "uA", "uB", 1000)
	_ = svc.Commit(ctx, fileID, "uA", "msg-del")
	objRow, _ := d.Get(ctx, `SELECT object_key FROM files WHERE id = ?`, fileID)
	objKey := objRow.Str("object_key")

	svc.DeleteByMessage(ctx, "msg-del", "uA")

	row, _ := d.Get(ctx, `SELECT id FROM files WHERE id = ?`, fileID)
	if row != nil {
		t.Fatal("file row should be gone")
	}
	if len(st.deleted) != 1 || st.deleted[0] != objKey {
		t.Fatalf("object not deleted from storage: %v", st.deleted)
	}
}

func TestDeleteByMessage_OnlyOwnerDeletes(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{size: 1000}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	ctx := context.Background()
	fileID, _, _ := svc.RequestUpload(ctx, "uA", "uB", 1000)
	_ = svc.Commit(ctx, fileID, "uA", "msg-del")

	svc.DeleteByMessage(ctx, "msg-del", "uB") // receiver, not sender

	row, _ := d.Get(ctx, `SELECT id FROM files WHERE id = ?`, fileID)
	if row == nil {
		t.Fatal("file should NOT be deleted by non-owner")
	}
}

func TestRequestUpload_NotContactsRejected(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	_, _ = d.Run(context.Background(), `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uC','CCCC-4444','C','pkC')`)
	svc := New(d, &mockStorage{}, 25*1024*1024, 10*1024*1024*1024)
	_, _, err := svc.RequestUpload(context.Background(), "uA", "uC", 1000)
	if err != ErrNotContacts {
		t.Fatalf("err = %v, want ErrNotContacts", err)
	}
}

func TestCleanupOrphans_ReapsStalePendingAndUnlinked(t *testing.T) {
	d := newDB(t)
	seedUsers(t, d)
	st := &mockStorage{}
	svc := New(d, st, 25*1024*1024, 10*1024*1024*1024)
	ctx := context.Background()
	old := time.Now().Unix() - reserveWindow - 10
	_, _ = d.Run(ctx, `INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, created_at) VALUES ('p1','uA','uB','kp1',10,'pending',?)`, old)
	_, _ = d.Run(ctx, `INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, created_at) VALUES ('c1','uA','uB','kc1',10,'committed',?)`, old)
	_, _ = d.Run(ctx, `INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, created_at) VALUES ('p2','uA','uB','kp2',10,'pending',?)`, time.Now().Unix())

	svc.CleanupOrphans(ctx)

	for _, id := range []string{"p1", "c1"} {
		if row, _ := d.Get(ctx, `SELECT id FROM files WHERE id = ?`, id); row != nil {
			t.Errorf("orphan %s should be reaped", id)
		}
	}
	if row, _ := d.Get(ctx, `SELECT id FROM files WHERE id = ?`, "p2"); row == nil {
		t.Error("fresh pending p2 must survive")
	}
	if len(st.deleted) != 2 {
		t.Errorf("expected 2 object deletions, got %d (%v)", len(st.deleted), st.deleted)
	}
}
