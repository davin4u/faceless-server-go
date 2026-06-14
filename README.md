# faceless-server-go

Drop-in Go port of `/server`. Same REST + Socket.IO wire protocol; opens the existing `messenger.db` in place. See `claude-plans/SERVER_GO_MIGRATION_PLAN.md` for the full protocol contract and `claude-plans/2026-05-03-server-go-migration-implementation.md` for the implementation plan.

## Quick start (dev)

```bash
cp .env.example .env
go run ./cmd/server
```

Server listens on :3000 by default. SQLite DB auto-created at `./data/messenger.db`.

## Tests

```bash
go test ./...
```

To include the Postgres integration tests:

```bash
docker run --rm -d --name pg-test -p 5433:5432 -e POSTGRES_PASSWORD=test postgres:16
TEST_PG_URL='postgres://postgres:test@localhost:5433/postgres?sslmode=disable' go test ./...
docker stop pg-test
```

## Build for Linux

```bash
./scripts/build-linux.sh
```

Output: `./dist/faceless-server` — static ELF 64-bit binary, no runtime dependencies.

## Logging

Logs are JSON by default. To see them human-readable:

```bash
LOG_FORMAT=text LOG_LEVEL=debug go run ./cmd/server
```

`LOG_ICE=true` switches every ICE candidate from DEBUG to INFO — useful when investigating call failures.

## File uploads (optional)

File uploads require an S3-compatible object store. Set `S3_BUCKET` (plus `S3_ENDPOINT`, `S3_REGION`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_USE_SSL`) in `.env` to enable the feature. Leave `S3_BUCKET` empty and the `/api/files` routes are simply not mounted — the server runs identically to before.

Key design points:
- The server stores only opaque E2E-encrypted blobs — no filenames, MIME types, or dimensions are recorded.
- Clients receive presigned PUT (upload) and GET (download) URLs directly to the object store; file bytes never pass through the app server.
- Storage is measured against a single global pool shared across all users (`MAX_STORAGE_TOTAL_GB`), with a per-file cap (`MAX_FILE_SIZE_MB`).
- Files are reclaimed when their associated message is deleted by the sender, or by the hourly orphan sweep that removes any blob whose message row has already been cleaned up.

## Cutover from Node

1. Stop the Node server: `systemctl stop faceless-server`
2. Copy the binary + .env to the VPS
3. Symlink or move `messenger.db` into `/opt/faceless-go/data/`
4. `systemctl start faceless-server-go`
5. Tail `journalctl -fu faceless-server-go` and run a smoke message + call

Rollback: `systemctl stop faceless-server-go && systemctl start faceless-server` — same DB file, no data loss.
