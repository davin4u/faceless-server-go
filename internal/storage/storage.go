// Package storage abstracts an S3-compatible object store behind a small
// interface so the files service can be unit-tested with a mock.
package storage

import (
	"context"
	"time"
)

// PutTTL / GetTTL bound how long a presigned URL is valid.
const (
	PutTTL = 15 * time.Minute
	GetTTL = 1 * time.Hour
)

// Storage is the minimal object-store surface the files service needs.
// Keys are opaque; values are E2E-encrypted ciphertext.
type Storage interface {
	// PresignPut returns a URL the client can PUT the object to.
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
	// PresignGet returns a URL the client can GET the object from.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Size returns the stored object's byte length (used to verify uploads).
	Size(ctx context.Context, key string) (int64, error)
	// Delete removes the object. Deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
}
