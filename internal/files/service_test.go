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
