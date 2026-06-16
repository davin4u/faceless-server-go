package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/avatars"
)

func newAvatarsHandler(t *testing.T) (*avatars.Service, http.Handler) {
	t.Helper()
	d := newSqlite(t)
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('uA','AAAA-2222','A','pkA')`)
	svc := avatars.New(d, &fakeStorage{size: 1000}, 2*1024*1024)
	return svc, NewAvatars(svc)
}

func TestAvatarRequestUpload_OK(t *testing.T) {
	_, h := newAvatarsHandler(t)
	body, _ := json.Marshal(map[string]any{"kind": "default", "sizeBytes": 1000})
	rr := callWithUser(h, "POST", "/request-upload", body, &auth.User{ID: "uA"})
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["avatarId"] == "" {
		t.Fatalf("missing avatarId: %v", resp)
	}
}

func TestAvatarRequestUpload_BadKind(t *testing.T) {
	_, h := newAvatarsHandler(t)
	body, _ := json.Marshal(map[string]any{"kind": "banner", "sizeBytes": 1000})
	rr := callWithUser(h, "POST", "/request-upload", body, &auth.User{ID: "uA"})
	if rr.Code != 400 {
		t.Fatalf("status = %d, want 400; body %s", rr.Code, rr.Body.String())
	}
}
