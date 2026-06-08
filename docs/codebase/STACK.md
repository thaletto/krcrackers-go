# Technology Stack

## Core Sections (Required)

### 1) Runtime Summary

| Area | Value | Evidence |
|------|-------|----------|
| Primary language | Go | `go.mod` |
| Runtime + version | Go 1.26.3 | `go.mod:3` |
| Package manager | Go modules | `go.mod`, `go.sum` |
| Module/build system | `go build` via Makefile | `Makefile:47` |

### 2) Production Frameworks and Dependencies

| Dependency | Version | Role in system | Evidence |
|------------|---------|----------------|----------|
| `net/http` (stdlib) | — | HTTP server, routing (Go 1.22+ pattern matching) | `main.go:116`, `Makefile:23` |
| `cloudflare/cloudflare-go/v7` | v7.4.0 | Cloudflare D1 HTTP API client | `database/d1.go:9` |
| `modernc.org/sqlite` | v1.51.0 | Pure-Go SQLite driver (no CGo) | `database/sqlite.go:11` |
| `pressly/goose/v3` | v3.27.1 | Migration version parser (`NumericComponent`) | `migrations/migrations.go:37` |
| `joho/godotenv` | v1.5.1 | `.env` / `.env.local` file loading | `config/config.go:8` |
| `aws/aws-lambda-go` | v1.54.0 | AWS Lambda runtime (secondary entry point) | `cmd/lambda/main.go:8` |
| `awslabs/aws-lambda-go-api-proxy` | v0.16.2 | Adapts `net/http` handler to Lambda API Gateway v2 | `cmd/lambda/main.go:9` |

### 3) Development Toolchain

| Tool | Purpose | Evidence |
|------|---------|----------|
| `make` | Build/dev task runner | `Makefile` |
| `air` (air-verse/air) | Hot reload on `.go` changes | `Makefile:30`, `.air.toml` |
| `wrangler` | Cloudflare D1 data export (prod→dev) | `Makefile:16` |
| `sqlite3` CLI | Load dumped SQL into local dev DB | `Makefile:19` |

### 4) Key Commands

```bash
make dev              # export prod D1 → local SQLite, start dev server
make run              # start dev server (go run .)
make stop             # kill dev server (frees :8080)
make watch            # hot reload (requires air)
make build            # go build ./...
make test             # go test ./... (no test files exist yet)
make migrate-up       # apply pending migrations
make migrate-down     # rollback latest migration
make migrate-status   # show applied/pending migrations
make build-lambda     # cross-compile for linux/arm64 Lambda
make deploy-lambda    # push binary to AWS Lambda
```

### 5) Environment and Config

- Config sources: `.env` (shared defaults), `.env.local` (personal overrides, gitignored)
- Required env vars:
  - `APP_ENV` — `development` (default) or `production`
  - `LOCAL_DB_PATH` — SQLite file path, default `.data/dev.sqlite`
  - `CLOUDFLARE_API_TOKEN` — D1 API token (production only)
  - `CLOUDFLARE_ACCOUNT_ID` — Cloudflare account ID (production only)
  - `CLOUDFLARE_DATABASE_ID` — D1 database ID (default `735027ae-2327-4561-8e62-538973817b06`)
  - `PORT` — HTTP port, default `8080`
- Deployment/runtime constraints: Go binary, no CGo (pure-Go SQLite via `modernc.org/sqlite`), cross-compilable to `linux/arm64` for Lambda

### 6) Evidence

- `go.mod` — module definition and all dependencies
- `Makefile` — all build/dev/deploy commands
- `.air.toml` — hot reload configuration
- `.env.example` — config template with all env vars

## Extended Sections (Optional)

Not needed for this small project.
