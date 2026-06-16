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

// Commit verifies the uploaded object size, marks the row committed, and deletes
// any previously-committed object of the same (user, kind).
func (s *Service) Commit(ctx context.Context, avatarID, userID string) error {
	row, err := s.d.Get(ctx,
		`SELECT object_key, size_bytes, kind FROM avatars WHERE id = ? AND user_id = ? AND status = 'pending'`,
		avatarID, userID)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrNotFound
	}
	actual, err := s.st.Size(ctx, row.Str("object_key"))
	if err != nil {
		return err
	}
	if actual != row.Int("size_bytes") {
		return ErrSizeMismatch
	}
	kind := row.Str("kind")
	// Remove the old committed avatar of the same kind (object + row), if any.
	if old, _ := s.d.Get(ctx,
		`SELECT id, object_key FROM avatars WHERE user_id = ? AND kind = ? AND status = 'committed'`, userID, kind); old != nil {
		_ = s.st.Delete(ctx, old.Str("object_key"))
		_, _ = s.d.Run(ctx, `DELETE FROM avatars WHERE id = ?`, old.Str("id"))
	}
	_, err = s.d.Run(ctx, `UPDATE avatars SET status = 'committed' WHERE id = ?`, avatarID)
	return err
}

// DownloadURL returns a presigned GET for a committed avatar, authorized only to
// the owner or an accepted contact of the owner.
func (s *Service) DownloadURL(ctx context.Context, avatarID, requesterID string) (string, error) {
	row, err := s.d.Get(ctx,
		`SELECT object_key, user_id FROM avatars WHERE id = ? AND status = 'committed'`, avatarID)
	if err != nil {
		return "", err
	}
	if row == nil {
		return "", ErrNotFound
	}
	owner := row.Str("user_id")
	if requesterID != owner {
		c, _ := s.d.Get(ctx,
			`SELECT 1 FROM contacts WHERE user_id = ? AND contact_id = ? AND status = 'accepted'`, owner, requesterID)
		if c == nil {
			return "", ErrForbidden
		}
	}
	return s.st.PresignGet(ctx, row.Str("object_key"), storage.GetTTL)
}

// DeleteCustom removes the caller's custom avatar (object + row), pending or committed.
func (s *Service) DeleteCustom(ctx context.Context, userID string) error {
	rows, err := s.d.All(ctx, `SELECT id, object_key FROM avatars WHERE user_id = ? AND kind = 'custom'`, userID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		_ = s.st.Delete(ctx, r.Str("object_key"))
		_, _ = s.d.Run(ctx, `DELETE FROM avatars WHERE id = ?`, r.Str("id"))
	}
	return nil
}

// CleanupOrphans reaps stale pending reservations. Called periodically.
func (s *Service) CleanupOrphans(ctx context.Context) {
	cutoff := time.Now().Unix() - reserveWindow
	rows, err := s.d.All(ctx, `SELECT id, object_key FROM avatars WHERE status = 'pending' AND created_at < ?`, cutoff)
	if err != nil {
		return
	}
	for _, r := range rows {
		_ = s.st.Delete(ctx, r.Str("object_key"))
		_, _ = s.d.Run(ctx, `DELETE FROM avatars WHERE id = ?`, r.Str("id"))
	}
}

// StartCleanup runs CleanupOrphans hourly until ctx is cancelled (initial run sync).
func (s *Service) StartCleanup(ctx context.Context) {
	s.CleanupOrphans(ctx)
	go func() {
		t := time.NewTicker(1 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.CleanupOrphans(ctx)
			}
		}
	}()
}

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
