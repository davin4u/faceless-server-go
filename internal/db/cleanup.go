package db

import (
	"context"
	"log/slog"
	"time"
)

// CleanupStaleMessages deletes undelivered messages older than 30 days.
// Returns number of rows deleted.
func CleanupStaleMessages(ctx context.Context, d DB) (int64, error) {
	cutoff := time.Now().Unix() - 30*86400
	// Unlink files whose undelivered message is about to be purged so the
	// files-service orphan sweep reclaims the S3 object + quota.
	_, _ = d.Run(ctx,
		`UPDATE files SET message_id = NULL
		 WHERE message_id IN (SELECT id FROM messages WHERE timestamp < ? AND delivered = 0)`, cutoff)
	res, err := d.Run(ctx, `DELETE FROM messages WHERE timestamp < ? AND delivered = 0`, cutoff)
	if err != nil {
		return 0, err
	}
	if res.Changes > 0 {
		slog.Info("cleanup.stale_messages.deleted", "count", res.Changes)
	}
	return res.Changes, nil
}

// CleanupRetiredCodes deletes retired codes older than 24h.
func CleanupRetiredCodes(ctx context.Context, d DB) (int64, error) {
	cutoff := time.Now().Unix() - 86400
	res, err := d.Run(ctx, `DELETE FROM retired_codes WHERE retired_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.Changes, nil
}

// StartCleanupJobs runs CleanupStaleMessages every 24h and CleanupRetiredCodes
// every 1h until ctx is cancelled. It performs an initial run synchronously.
func StartCleanupJobs(ctx context.Context, d DB) {
	if _, err := CleanupStaleMessages(ctx, d); err != nil {
		slog.Error("cleanup.stale_messages.error", "err", err)
	}
	if _, err := CleanupRetiredCodes(ctx, d); err != nil {
		slog.Error("cleanup.retired_codes.error", "err", err)
	}
	go ticker(ctx, 24*time.Hour, func() { _, _ = CleanupStaleMessages(ctx, d) })
	go ticker(ctx, 1*time.Hour, func() { _, _ = CleanupRetiredCodes(ctx, d) })
}

func ticker(ctx context.Context, interval time.Duration, fn func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}
