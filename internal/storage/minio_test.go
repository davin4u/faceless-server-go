package storage

import (
	"testing"
	"time"
)

// NewMinio must construct a client without dialing S3 (minio-go is lazy).
func TestNewMinio_Constructs(t *testing.T) {
	s, err := NewMinio("s3.example.com", "us-east-1", "access", "secret", true, "bucket")
	if err != nil {
		t.Fatalf("NewMinio error: %v", err)
	}
	if s == nil {
		t.Fatal("NewMinio returned nil")
	}
}

// Storage is the interface the files service depends on.
var _ Storage = (*Minio)(nil)

func TestPresignTTLConstantsSane(t *testing.T) {
	if PutTTL <= 0 || GetTTL <= 0 || PutTTL > 7*24*time.Hour || GetTTL > 7*24*time.Hour {
		t.Fatalf("TTLs out of range: put=%v get=%v", PutTTL, GetTTL)
	}
}
