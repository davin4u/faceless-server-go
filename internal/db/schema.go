package db

import (
	"context"
	"strings"
)

func InitSchema(ctx context.Context, d DB) error {
	tables := tablesDDL(d)
	for _, stmt := range splitStmts(tables) {
		if err := d.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	if d.Dialect() == "sqlite" {
		if err := runSqliteMigrations(ctx, d); err != nil {
			return err
		}
	}

	indexes := indexesDDL()
	for _, stmt := range splitStmts(indexes) {
		// Index may already exist on a re-run; ignore that error class.
		_ = d.Exec(ctx, stmt)
	}
	return nil
}

func tablesDDL(d DB) string {
	now := d.NowEpoch()
	return `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  contact_code TEXT UNIQUE NOT NULL,
  display_name TEXT NOT NULL,
  public_key TEXT NOT NULL,
  chat_public_key TEXT,
  username TEXT,
  password_hash TEXT,
  created_at INTEGER NOT NULL DEFAULT (` + now + `)
);

CREATE TABLE IF NOT EXISTS contacts (
  user_id TEXT NOT NULL,
  contact_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending', 'accepted', 'blocked')),
  created_at INTEGER NOT NULL DEFAULT (` + now + `),
  PRIMARY KEY (user_id, contact_id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (contact_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  sender_id TEXT NOT NULL,
  receiver_id TEXT NOT NULL,
  ciphertext TEXT NOT NULL,
  nonce TEXT NOT NULL,
  timestamp INTEGER NOT NULL DEFAULT (` + now + `),
  delivered INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (sender_id) REFERENCES users(id),
  FOREIGN KEY (receiver_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS pending_events (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload TEXT NOT NULL,
  timestamp INTEGER NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS retired_codes (
  code TEXT PRIMARY KEY,
  retired_at INTEGER NOT NULL DEFAULT (` + now + `)
);

CREATE TABLE IF NOT EXISTS daily_stats (
  date TEXT PRIMARY KEY,
  messages_sent INTEGER DEFAULT 0,
  audio_calls INTEGER DEFAULT 0,
  video_calls INTEGER DEFAULT 0,
  completed_calls INTEGER DEFAULT 0,
  total_call_duration_seconds INTEGER DEFAULT 0,
  registrations INTEGER DEFAULT 0
);
`
}

func indexesDDL() string {
	return `
CREATE INDEX IF NOT EXISTS idx_users_contact_code ON users(contact_code);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_public_key ON users(public_key);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_chat_public_key ON users(chat_public_key);
CREATE INDEX IF NOT EXISTS idx_messages_receiver_delivered ON messages(receiver_id, delivered);
CREATE INDEX IF NOT EXISTS idx_pending_events_user ON pending_events(user_id);
`
}

func splitStmts(blob string) []string {
	var out []string
	for _, s := range strings.Split(blob, ";") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s+";")
	}
	return out
}
