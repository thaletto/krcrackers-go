# Technology Stack

## Runtime

| Area | Value |
|------|-------|
| Primary language | Go |
| Runtime + version | Go 1.26.3 |
| Package manager | Go modules |
| Module/build system | `go build` via Makefile |

## Production Dependencies

| Dependency | Version | Role |
|------------|---------|------|
| `net/http` (stdlib) | — | HTTP server, routing (Go 1.22+ pattern matching) |
| `cloudflare/cloudflare-go/v7` | v7.4.0 | Cloudflare D1 HTTP API client |
| `modernc.org/sqlite` | v1.51.0 | Pure-Go SQLite driver (no CGo) |
| `pressly/goose/v3` | v3.27.1 | Migration version parser |
| `joho/godotenv` | v1.5.1 | `.env` / `.env.local` file loading |
| `golang-jwt/jwt/v5` | — | JWT generation and validation |
| `golang.org/x/crypto` | — | bcrypt password hashing |
| `meilisearch/meilisearch-go` | v0.36.3 | Meilisearch client |
| `aws/aws-sdk-go-v2` | — | S3-compatible R2 file uploads |
| `jung-kurt/gofpdf` | — | PDF invoice generation |
| `aws/aws-lambda-go` | v1.54.0 | AWS Lambda runtime (secondary) |
| `awslabs/aws-lambda-go-api-proxy` | v0.16.2 | Lambda API Gateway adapter |

## Development Toolchain

| Tool | Purpose |
|------|---------|
| `make` | Build/dev task runner |
| `air` (air-verse/air) | Hot reload on `.go` changes |
| `wrangler` | Cloudflare D1 data export (prod→dev) |
| `sqlite3` CLI | Load dumped SQL into local dev DB |

## Key Commands

```bash
make dev              # export prod D1 → local SQLite, start dev server
make run              # start dev server (go run .)
make stop             # kill dev server (frees :8080)
make watch            # hot reload (requires air)
make build            # go build ./...
make test             # go test ./... (no test files exist yet)
make test-endpoints   # run 64 endpoint integration tests
make migrate-up       # apply pending migrations
make migrate-down     # rollback latest migration
make migrate-status   # show applied/pending migrations
make build-lambda     # cross-compile for linux/arm64 Lambda
make deploy-lambda    # push binary to AWS Lambda
```

## Environment Config

- Config sources: `.env` (shared defaults), `.env.local` (personal overrides, gitignored)
- `APP_ENV` defaults to `"development"` unless `CLOUDFLARE_API_TOKEN` is set
- `JWT_SECRET` is required; server panics at startup if empty
