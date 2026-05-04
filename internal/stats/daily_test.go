package stats

import (
	"context"
	"testing"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

func newSqlite(t *testing.T) db.DB {
	t.Helper()
	d, err := db.NewSqlite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.InitSchema(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestIncrementDaily_InsertThenUpdate(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)

	if err := IncrementDaily(ctx, d, ColMessagesSent, 1); err != nil {
		t.Fatal(err)
	}
	if err := IncrementDaily(ctx, d, ColMessagesSent, 2); err != nil {
		t.Fatal(err)
	}
	if err := IncrementDaily(ctx, d, ColAudioCalls, 1); err != nil {
		t.Fatal(err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	row, err := d.Get(ctx, `SELECT messages_sent, audio_calls FROM daily_stats WHERE date = ?`, today)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil {
		t.Fatal("row missing")
	}
	if row.Int("messages_sent") != 3 {
		t.Errorf("messages_sent = %d", row.Int("messages_sent"))
	}
	if row.Int("audio_calls") != 1 {
		t.Errorf("audio_calls = %d", row.Int("audio_calls"))
	}
}

func TestIncrementDaily_RejectsBadColumn(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t)
	if err := IncrementDaily(ctx, d, "evil; DROP TABLE users; --", 1); err == nil {
		t.Error("bad column should error, not run SQL")
	}
}
