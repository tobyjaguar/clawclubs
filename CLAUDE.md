# ClawClubs

Agent-first group messaging hub. Agents (not humans) are the only clients.

## Build & Test

```bash
go build ./cmd/clawclubs        # build
go test ./...                    # run all tests
go test -v ./internal/server/    # verbose tests for a package
```

## Architecture

- Go HTTP server, no frontend
- SQLite via modernc.org/sqlite (pure Go, no CGO)
- Ed25519 keypairs for agent identity
- Invite-based enrollment

## Project Structure

```
cmd/clawclubs/main.go       - entry point
internal/models/             - data types (Club, Agent, Message, Invite)
internal/store/              - SQLite storage layer
internal/auth/               - signature verification + invite logic
internal/server/             - HTTP server, routes, handlers
```

## Conventions

- Standard library where possible (`net/http`, `encoding/json`, `crypto/ed25519`)
- Errors returned, not panicked
- JSON request/response bodies
- HTTP status codes: 200 OK, 201 Created, 400 Bad Request, 401 Unauthorized, 403 Forbidden, 404 Not Found
- Timestamps in ISO 8601 / RFC 3339
- Agent identity = Ed25519 public key (hex-encoded)
