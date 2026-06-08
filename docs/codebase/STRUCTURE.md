# Codebase Structure

## Core Sections (Required)

### 1) Top-Level Map

| Path | Purpose | Evidence |
|------|---------|----------|
| `main.go` | Server entry: handler wiring, lifecycle, `migrate` subcommand | `main.go` |
| `config/` | Environment loading (godotenv, `.env.local` override) | `config/config.go` |
| `database/` | `DB` + `Tx` + `Row` interfaces and adapters (SQLite, D1) | `database/database.go` |
| `server/` | Shared HTTP helpers (`WriteJSON`, `WriteError`, `WithLogging`) | `server/server.go` |
| `services/` | Business domains: `products/`, `orders/` | `services/` |
| `migrations/` | Embedded SQL migrations (goose format) + runner | `migrations/migrations.go` |
| `cmd/lambda/` | AWS Lambda entry point (secondary) | `cmd/lambda/main.go` |
| `wrangler.toml` | Cloudflare D1 binding for `make dev-db` | `wrangler.toml` |
| `Makefile` | Dev/build/deploy task runner | `Makefile` |
| `.air.toml` | Hot reload config | `.air.toml` |
| `.env.example` | Config template | `.env.example` |
| `docs/` | Existing docs: configuration, API, database, migrations, project-structure, deployment | `docs/` |

### 2) Entry Points

- Main runtime entry: `main.go`
- Secondary entry point: `cmd/lambda/main.go` (AWS Lambda)
- How entry is selected: `main.go` checks `os.Args[1] == "migrate"` for migration subcommand, otherwise starts HTTP server. `cmd/lambda/main.go` is compiled separately via `make build-lambda`.

### 3) Module Boundaries

| Boundary | What belongs here | What must not be here |
|----------|-------------------|------------------------|
| `config/` | Env loading, `Config` struct construction | Business logic, HTTP handlers |
| `database/` | `DB`/`Tx`/`Row` interfaces, SQLite/D1 adapters, `Config`, factory | SQL queries (belong in repositories) |
| `server/` | HTTP response helpers, request logging middleware | Business logic, data access |
| `services/products/` | Product HTTP handlers + repository (SQL, row mapping, transactions) | Cross-domain logic |
| `services/orders/` | Order HTTP handlers + repository (SQL, row mapping, transactions) | Cross-domain logic |
| `migrations/` | SQL file parsing, version tracking, up/down/status commands | Business logic |
| `cmd/lambda/` | Lambda bootstrap, `httpadapter` wiring | HTTP handlers (reuse `services/`) |

### 4) Naming and Organization Rules

- File naming pattern: lowercase, short descriptive names (`database.go`, `sqlite.go`, `repository.go`)
- Directory organization pattern: domain-based (`services/products/`, `services/orders/`)
- Package naming: lowercase singular (`products`, `orders`, `database`, `server`, `config`, `migrations`)
- Import aliasing: none; all imports use package names directly

### 5) Evidence

- `main.go` — entry point with handler wiring
- `cmd/lambda/main.go` — Lambda entry point
- `database/database.go` — `DB` interface and factory
- `services/products/main.go` — example service with handlers
- `services/products/repository.go` — example repository
- `docs/project-structure.md` — existing structure documentation

## Extended Sections (Optional)

Not needed for this small project.
