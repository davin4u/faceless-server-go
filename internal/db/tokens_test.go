package db

import (
	"context"
	"testing"
)

func newTestDB(t *testing.T) DB {
	t.Helper()
	d, err := NewSqlite(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := InitSchema(context.Background(), d); err != nil {
		t.Fatalf("schema: %v", err)
	}
	_, _ = d.Run(context.Background(),
		`INSERT INTO users (id, contact_code, display_name, public_key, created_at) VALUES ('u1','AAAA-BBBB','U One','pk1',0)`)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestUpsertAndGetTokens(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)
	if err := UpsertDeviceToken(ctx, d, "u1", "tokA", "android"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := UpsertDeviceToken(ctx, d, "u1", "tokA", "android"); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	toks, err := GetUserTokens(ctx, d, "u1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(toks) != 1 || toks[0] != "tokA" {
		t.Fatalf("want [tokA], got %v", toks)
	}
}

func TestDeleteToken(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)
	_ = UpsertDeviceToken(ctx, d, "u1", "tokA", "android")
	_ = UpsertDeviceToken(ctx, d, "u1", "tokB", "android")
	if err := DeleteToken(ctx, d, "tokA"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	toks, _ := GetUserTokens(ctx, d, "u1")
	if len(toks) != 1 || toks[0] != "tokB" {
		t.Fatalf("want [tokB], got %v", toks)
	}
}

func TestDeleteDeviceTokenForUser(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)
	_ = UpsertDeviceToken(ctx, d, "u1", "tokA", "android")
	if err := DeleteDeviceToken(ctx, d, "u1", "tokA"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	toks, _ := GetUserTokens(ctx, d, "u1")
	if len(toks) != 0 {
		t.Fatalf("want empty, got %v", toks)
	}
}
