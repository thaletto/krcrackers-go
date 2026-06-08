# Project Structure

```
.
├── main.go                    # server entry, handler wiring, lifecycle, migrate subcommand
├── config/config.go           # env loading (godotenv, .env.local override)
├── database/
│   ├── database.go            # DB + Tx interfaces, Row typed-accessor seam, Config, factory
│   ├── d1.go                  # Cloudflare D1 backend (best-effort Tx adapter)
│   └── sqlite.go              # local SQLite backend with real Tx support
├── server/
│   └── server.go              # shared HTTP helpers (WriteJSON, WriteError, WithLogging)
├── services/
│   ├── orders/
│   │   ├── main.go            # thin HTTP handlers (delegate to repository)
│   │   └── repository.go      # SQL, row mapping, transactions
│   └── products/
│       ├── main.go            # thin HTTP handlers (delegate to repository)
│       └── repository.go      # SQL, row mapping
├── migrations/
│   ├── migrations.go          # embedded SQL runner; goose.NumericComponent for version parsing
│   └── 0001_init.sql          # goose-format schema (Up / Down)
├── wrangler.toml              # D1 binding
├── Makefile                   # dev-db / run / dev / watch / migrate-*
├── .air.toml                  # hot reload config
└── .env.example               # config template
```

## Architecture

```
HTTP Handler (parse, validate, respond)
    ↓ calls
Repository (SQL, row mapping, transactions)
    ↓ uses
database.DB + Tx (seam — adapters: SQLite, D1)
```

- **Handlers** are thin HTTP adapters. They decode requests, call the repository, and write responses.
- **Repositories** own the SQL and row mapping. They are the seam between HTTP and the database.
- **`database.Tx`** provides atomicity. SQLite wraps `*sql.Tx` (real atomicity). D1 buffers statements and executes on commit (best-effort; not truly atomic).

Handler wiring lives in `main.go` and `cmd/lambda/main.go`. The `server` package provides shared HTTP helpers (`WriteJSON`, `WriteError`, `WithLogging`) used by both entry points and services.

The `database.DB` interface and the `Row` typed-accessor interface are both backend-agnostic. Adding another backend (Postgres, in-memory for tests, etc.) is one new file that implements `Query` / `Execute` / `Begin` / `Close` / `Row`, and a switch case in `database.New`.
