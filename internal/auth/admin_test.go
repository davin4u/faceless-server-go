package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAdminBearer_PassesWithCorrectToken(t *testing.T) {
	mw := RequireAdminBearer("super-secret")
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer super-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called || rr.Code != 200 {
		t.Errorf("want pass, got status=%d", rr.Code)
	}
}

func TestRequireAdminBearer_RejectsWrongToken(t *testing.T) {
	mw := RequireAdminBearer("super-secret")
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("should not pass")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer wrong-token-of-similar-length")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestRequireAdminBearer_503WhenUnconfigured(t *testing.T) {
	mw := RequireAdminBearer("")
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("should not pass")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 503 {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestRequireAdminBearer_RejectsMissingHeader(t *testing.T) {
	mw := RequireAdminBearer("super-secret")
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal() }))
	req := httptest.NewRequest("GET", "/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Errorf("status = %d", rr.Code)
	}
}
