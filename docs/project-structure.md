# Project Structure

```
.
├── main.go                    # server entry, lifecycle, migrate subcommand
├── config/config.go           # env loading (godotenv, .env.local override)
├── database/
│   ├── database.go            # DB interface, Row typed-accessor seam, Config tagged union, factory
│   ├── d1.go                  # Cloudflare D1 backend
│   └── sqlite.go              # local SQLite backend (modernc.org/sqlite)
├── server/
│   └── handler.go             # route registration, logging middleware, JSON helpers
├── services/
│   ├── orders/main.go         # net/http handlers + typed Row accessors
│   └── products/main.go       # net/http handlers + typed Row accessors
├── migrations/
│   ├── migrations.go          # embedded SQL runner; goose.NumericComponent for version parsing
│   └── 0001_init.sql          # goose-format schema (Up / Down)
├── wrangler.toml              # D1 binding
├── Makefile                   # dev-db / run / dev / watch / migrate-*
├── .air.toml                  # hot reload config
└── .env.example               # config template
```

The `database.DB` interface and the `Row` typed-accessor interface are both backend-agnostic. Adding another backend (Postgres, in-memory for tests, etc.) is one new file that implements `Query` / `Execute` / `Close` / `Row`, and a switch case in `database.New`.
