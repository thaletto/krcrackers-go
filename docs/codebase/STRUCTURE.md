# Codebase Structure

## Top-Level Map

| Path | Purpose |
|------|---------|
| `main.go` | Server entry: handler wiring, lifecycle, `migrate` subcommand |
| `config/` | Environment loading (godotenv, `.env.local` override) |
| `database/` | `DB` + `Tx` + `Row` interfaces and adapters (SQLite, D1) |
| `server/` | Shared HTTP helpers (`WriteJSON`, `WriteError`, `WithLogging`) |
| `eventbus/` | In-memory pub/sub event bus for cross-service communication |
| `adapters/` | Shared adapter implementations bridging service interfaces |
| `errors/` | Sentinel error types (`ErrNotFound`) |
| `services/` | Business domains (see below) |
| `migrations/` | Embedded SQL migrations (goose format) + runner |
| `scripts/` | Integration test scripts |
| `cmd/lambda/` | AWS Lambda entry point (secondary) |
| `docs/` | Documentation and HTML docs |

## Entry Points

- **Main runtime**: `main.go` — starts HTTP server or runs migration subcommand
- **Lambda**: `cmd/lambda/main.go` — compiled separately via `make build-lambda`
- **Selection**: `main.go` checks `os.Args[1] == "migrate"` for migration subcommand

## Services

| Service | Files | Description |
|---------|-------|-------------|
| `services/auth/` | `main.go`, `repository.go`, `jwt.go`, `google.go`, `password.go`, `middleware.go` | JWT auth, Google ID token, refresh tokens, middleware |
| `services/customers/` | `main.go`, `repository.go` | Customer profiles and address management |
| `services/products/` | `main.go`, `repository.go` | Product CRUD, search/filter/sort, event publishing |
| `services/orders/` | `main.go`, `repository.go` | Order lifecycle, checkout, admin management |
| `services/search/` | `main.go`, `subscriber.go` | Meilisearch integration via event subscriber |
| `services/uploads/` | `main.go` | Cloudflare R2 file uploads |
| `services/notifications/` | `main.go`, `subscriber.go` | WhatsApp Cloud API via event subscriber |
| `services/invoices/` | `main.go` | PDF invoice generation |

## Module Boundaries

| Boundary | What belongs here | What must not be here |
|----------|-------------------|------------------------|
| `config/` | Env loading, `Config` struct construction | Business logic, HTTP handlers |
| `database/` | `DB`/`Tx`/`Row` interfaces, SQLite/D1 adapters | SQL queries (belong in repositories) |
| `server/` | HTTP response helpers, request logging middleware | Business logic, data access |
| `eventbus/` | Event bus interface and in-memory implementation | Business logic, data access |
| `adapters/` | Interface adapters bridging service boundaries | Business logic |
| `services/*/` | HTTP handlers + repository per domain | Cross-domain logic |

## Naming Conventions

- Files: lowercase, short descriptive (`database.go`, `sqlite.go`, `repository.go`)
- Packages: lowercase singular (`products`, `orders`, `database`, `server`)
- Exported: PascalCase (`NewService`, `NewRepository`, `WriteJSON`)
- Unexported: camelCase (`rowToProduct`, `validateProductInput`)
