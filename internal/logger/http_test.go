package logger

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogger_LogsStartAndEnd(t *testing.T) {
	var buf bytes.Buffer
	Init(Config{Level: "debug", Format: "json", Out: &buf})

	mux := http.NewServeMux()
	mux.HandleFunc("/x", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	h := RequestLogger(mux)

	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines (start + end), got %d: %v", len(lines), lines)
	}
	var start, end map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &start); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &end); err != nil {
		t.Fatal(err)
	}
	if start["msg"] != "http.request.start" {
		t.Errorf("start msg = %v", start["msg"])
	}
	if end["msg"] != "http.request.end" {
		t.Errorf("end msg = %v", end["msg"])
	}
	if start["request_id"] == nil || start["request_id"] == "" {
		t.Errorf("missing request_id in start: %v", start)
	}
	if start["request_id"] != end["request_id"] {
		t.Errorf("request_id should match across start and end: %v vs %v", start["request_id"], end["request_id"])
	}
	if int(end["status"].(float64)) != 204 {
		t.Errorf("status = %v, want 204", end["status"])
	}
}
