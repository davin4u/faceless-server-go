package socketio

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/davin4u/faceless-server-go/internal/db"
)

func mountServer(t *testing.T, d db.DB) (string, *Server, func()) {
	t.Helper()
	s := New(d, false)
	mux := http.NewServeMux()
	mux.Handle("/socket.io/", s.Handler())
	hs := httptest.NewServer(mux)
	return hs.URL, s, func() { hs.Close(); _ = s.Close(context.Background()) }
}

func TestSocketIO_AuthHandshakeAcceptsValidSignature(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available")
	}

	d, err := db.NewSqlite(t.TempDir() + "/io.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.InitSchema(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	pub, sec, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	secB64 := base64.StdEncoding.EncodeToString(sec)

	if _, err := d.Run(context.Background(),
		`INSERT INTO users (id, contact_code, display_name, public_key, chat_public_key) VALUES (?, ?, ?, ?, ?)`,
		"u1", "AAAA-2222", "Alice", pubB64, "ck"); err != nil {
		t.Fatal(err)
	}

	url, _, cleanup := mountServer(t, d)
	defer cleanup()

	_, file, _, _ := runtime.Caller(0)
	probe := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "sio-client", "probe.js")
	cmd := exec.Command("node", probe, url, pubB64, secB64)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("probe exited: %v stderr=%s", err, stderr.String())
	}

	gotConnect := false
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err == nil && ev["event"] == "connect" {
			gotConnect = true
			break
		}
	}
	if !gotConnect {
		t.Errorf("did not see connect event; stdout=%q", stdout.String())
	}
}
