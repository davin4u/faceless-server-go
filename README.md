# faceless-server-go

Drop-in Go port of `/server`. Same REST + Socket.IO wire protocol; opens the existing `messenger.db` in place.

## Run

```bash
cp .env.example .env
go run ./cmd/server
```

## Build (Linux deploy target)

```bash
GOOS=linux GOARCH=amd64 go build -o faceless-server ./cmd/server
```

See `claude-plans/SERVER_GO_MIGRATION_PLAN.md` for the full protocol contract.
