// Package stats writes incremental counters into the daily_stats table.
//
// Both SQLite and PostgreSQL support `INSERT ... ON CONFLICT(date) DO UPDATE`
// so the same SQL works in both dialects.
package stats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

// Allowed counter column names — anything else is rejected to keep the SQL
// safe (the column name is interpolated into the SQL string).
const (
	ColMessagesSent     = "messages_sent"
	ColAudioCalls       = "audio_calls"
	ColVideoCalls       = "video_calls"
	ColCompletedCalls   = "completed_calls"
	ColCallDurationSecs = "total_call_duration_seconds"
	ColRegistrations    = "registrations"
)

var allowed = map[string]bool{
	ColMessagesSent: true, ColAudioCalls: true, ColVideoCalls: true,
	ColCompletedCalls: true, ColCallDurationSecs: true, ColRegistrations: true,
}

// IncrementDaily upserts daily_stats for today (UTC) and adds delta to the column.
func IncrementDaily(ctx context.Context, d db.DB, column string, delta int64) error {
	if !allowed[column] {
		return errors.New("stats: invalid column " + column)
	}
	today := time.Now().UTC().Format("2006-01-02")
	q := fmt.Sprintf(
		`INSERT INTO daily_stats (date, %[1]s) VALUES (?, ?) ON CONFLICT(date) DO UPDATE SET %[1]s = daily_stats.%[1]s + ?`,
		column,
	)
	_, err := d.Run(ctx, q, today, delta, delta)
	return err
}
