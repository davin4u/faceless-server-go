// Package files combines the DB and object storage to enforce a single global
// byte quota, reserve/commit uploads, authorize downloads, and reap orphans.
// It is the only place that knows an upload's lifecycle; routes and the socket
// layer call into it. The server stores only an opaque object key + byte size —
// never filenames, MIME types, or dimensions (those stay E2E-encrypted client-side).
package files

import (
	"context"
	"errors"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/storage"
	"github.com/google/uuid"
)

var (
	ErrTooLarge     = errors.New("file exceeds per-file size limit")
	ErrStorageFull  = errors.New("global storage pool is full")
	ErrNotFound     = errors.New("file not found")
	ErrForbidden    = errors.New("not authorized for this file")
	ErrSizeMismatch = errors.New("uploaded size does not match declared size")
)

// reserveWindow is how long a pending reservation counts against the quota
// before the cleanup sweep can reclaim it.
const reserveWindow = 3600 // seconds

type Service struct {
	d        db.DB
	st       storage.Storage
	maxFile  int64
	maxTotal int64
}

func New(d db.DB, st storage.Storage, maxFile, maxTotal int64) *Service {
	return &Service{d: d, st: st, maxFile: maxFile, maxTotal: maxTotal}
}

// usedBytes = committed bytes + live (not-yet-expired) pending reservations.
func (s *Service) usedBytes(ctx context.Context) (int64, error) {
	cutoff := time.Now().Unix() - reserveWindow
	row, err := s.d.Get(ctx,
		`SELECT COALESCE(SUM(size_bytes), 0) AS total FROM files
		 WHERE status = 'committed' OR (status = 'pending' AND created_at >= ?)`, cutoff)
	if err != nil {
		return 0, err
	}
	return row.Int("total"), nil
}

// RequestUpload validates the size against the per-file limit and the global
// pool, reserves a pending row, and returns the fileID + a presigned PUT URL.
func (s *Service) RequestUpload(ctx context.Context, senderID, receiverID string, size int64) (string, string, error) {
	if size <= 0 || size > s.maxFile {
		return "", "", ErrTooLarge
	}
	used, err := s.usedBytes(ctx)
	if err != nil {
		return "", "", err
	}
	if used+size > s.maxTotal {
		return "", "", ErrStorageFull
	}
	fileID := uuid.NewString()
	objectKey := uuid.NewString()
	if _, err := s.d.Run(ctx,
		`INSERT INTO files (id, sender_id, receiver_id, object_key, size_bytes, status, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		fileID, senderID, receiverID, objectKey, size, time.Now().Unix()); err != nil {
		return "", "", err
	}
	url, err := s.st.PresignPut(ctx, objectKey, storage.PutTTL)
	if err != nil {
		return "", "", err
	}
	return fileID, url, nil
}

// Commit verifies the uploaded object's size matches what was declared, then
// marks the reservation committed and links it to the chat message. Only the
// original sender may commit their own pending file.
func (s *Service) Commit(ctx context.Context, fileID, senderID, messageID string) error {
	row, err := s.d.Get(ctx,
		`SELECT object_key, size_bytes FROM files WHERE id = ? AND sender_id = ? AND status = 'pending'`,
		fileID, senderID)
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
	_, err = s.d.Run(ctx,
		`UPDATE files SET status = 'committed', message_id = ? WHERE id = ?`, messageID, fileID)
	return err
}

// DownloadURL returns a presigned GET URL for a committed file, but only to its
// sender or receiver.
func (s *Service) DownloadURL(ctx context.Context, fileID, requesterID string) (string, error) {
	row, err := s.d.Get(ctx,
		`SELECT object_key, sender_id, receiver_id FROM files WHERE id = ? AND status = 'committed'`, fileID)
	if err != nil {
		return "", err
	}
	if row == nil {
		return "", ErrNotFound
	}
	if requesterID != row.Str("sender_id") && requesterID != row.Str("receiver_id") {
		return "", ErrForbidden
	}
	return s.st.PresignGet(ctx, row.Str("object_key"), storage.GetTTL)
}
