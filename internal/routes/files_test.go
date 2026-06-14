package routes

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/files"
)

type fakeStorage struct{ size int64 }

func (f *fakeStorage) PresignPut(context.Context, string, time.Duration) (string, error) {
	return "https://put/obj", nil
}
func (f *fakeStorage) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "https://get/obj", nil
}
func (f *fakeStorage) Size(context.Context, string) (int64, error) { return f.size, nil }
func (f *fakeStorage) Delete(context.Context, string) error        { return nil }

func newFilesHandler(t *testing.T, size int64) (*files.Service, db.DB) {
	t.Helper()
	d := newSqlite(t)
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uA','AAAA-2222','A','pkA')`)
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uB','BBBB-3333','B','pkB')`)
	svc := files.New(d, &fakeStorage{size: size}, 25*1024*1024, 10*1024*1024*1024)
	return svc, d
}

func TestFiles_RequestUpload200(t *testing.T) {
	svc, _ := newFilesHandler(t, 1000)
	h := NewFiles(svc)
	uA := &auth.User{ID: "uA"}
	body, _ := json.Marshal(map[string]any{"sizeBytes": 1000, "to": "uB"})
	rr := callWithUser(h, "POST", "/request-upload", body, uA)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["fileId"] == "" || resp["uploadUrl"] == "" {
		t.Fatalf("missing fields: %v", resp)
	}
}

func TestFiles_RequestUpload413TooLarge(t *testing.T) {
	svc, _ := newFilesHandler(t, 1000)
	h := NewFiles(svc)
	body, _ := json.Marshal(map[string]any{"sizeBytes": 26 * 1024 * 1024, "to": "uB"})
	rr := callWithUser(h, "POST", "/request-upload", body, &auth.User{ID: "uA"})
	if rr.Code != 413 {
		t.Fatalf("status = %d, want 413", rr.Code)
	}
}

func TestFiles_CommitThenDownload(t *testing.T) {
	svc, _ := newFilesHandler(t, 1000)
	h := NewFiles(svc)
	uA := &auth.User{ID: "uA"}

	rb, _ := json.Marshal(map[string]any{"sizeBytes": 1000, "to": "uB"})
	rr := callWithUser(h, "POST", "/request-upload", rb, uA)
	var up map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &up)

	cb, _ := json.Marshal(map[string]string{"fileId": up["fileId"], "messageId": "m1"})
	rr = callWithUser(h, "POST", "/commit", cb, uA)
	if rr.Code != 200 {
		t.Fatalf("commit status %d body %s", rr.Code, rr.Body.String())
	}

	rr = callWithUser(h, "GET", "/"+up["fileId"]+"/download-url", nil, &auth.User{ID: "uB"})
	if rr.Code != 200 {
		t.Fatalf("download status %d body %s", rr.Code, rr.Body.String())
	}
	var dl map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &dl)
	if dl["url"] == "" {
		t.Fatalf("missing url: %v", dl)
	}
}
