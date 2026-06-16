// Package avatars stores E2E-encrypted user avatars. Unlike files, an avatar is
// a per-user profile attribute (one 'default' + optional 'custom'), not tied to
// a message or recipient. The server only ever holds ciphertext + an opaque key.
package avatars

import (
	"context"
	"errors"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/storage"
	"github.com/google/uuid"
)

var (
	ErrTooLarge     = errors.New("avatar exceeds size limit")
	ErrNotFound     = errors.New("avatar not found")
	ErrForbidden    = errors.New("not authorized for this avatar")
	ErrSizeMismatch = errors.New("uploaded size does not match declared size")
	ErrBadKind      = errors.New("kind must be 'default' or 'custom'")
)

const reserveWindow = 3600 // seconds; stale pending rows reclaimable after this

type Service struct {
	d        db.DB
	st       storage.Storage
	maxBytes int64
}

func New(d db.DB, st storage.Storage, maxBytes int64) *Service {
	return &Service{d: d, st: st, maxBytes: maxBytes}
}

func validKind(k string) bool { return k == "default" || k == "custom" }

// RequestUpload reserves a pending avatar row for (userID, kind), replacing any
// existing pending row of that kind, and returns the avatarID + presigned PUT URL.
func (s *Service) RequestUpload(ctx context.Context, userID, kind string, size int64) (string, string, error) {
	if !validKind(kind) {
		return "", "", ErrBadKind
	}
	if size <= 0 || size > s.maxBytes {
		return "", "", ErrTooLarge
	}
	// Drop any prior *pending* row for this (user, kind) so we never leak reservations.
	if old, _ := s.d.Get(ctx,
		`SELECT object_key FROM avatars WHERE user_id = ? AND kind = ? AND status = 'pending'`, userID, kind); old != nil {
		_ = s.st.Delete(ctx, old.Str("object_key"))
		_, _ = s.d.Run(ctx, `DELETE FROM avatars WHERE user_id = ? AND kind = ? AND status = 'pending'`, userID, kind)
	}
	avatarID := uuid.NewString()
	objectKey := uuid.NewString()
	if _, err := s.d.Run(ctx,
		`INSERT INTO avatars (id, user_id, kind, object_key, size_bytes, status, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		avatarID, userID, kind, objectKey, size, time.Now().Unix()); err != nil {
		return "", "", err
	}
	url, err := s.st.PresignPut(ctx, objectKey, storage.PutTTL)
	if err != nil {
		return "", "", err
	}
	return avatarID, url, nil
}
