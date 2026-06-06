# KR Crackers Go Backend

A small Go HTTP API for KR Crackers. Backed by Cloudflare D1 in production, local SQLite in development, with auto-generated OpenAPI docs via [huma](https://github.com/danielgtaylor/huma).

## Stack

- **Go 1.26+**, std `net/http` (Go 1.22+ pattern routing) via `humago` adapter
- **Cloudflare D1** (SQLite at the edge) in production, **local SQLite** via `modernc.org/sqlite` in dev
- **huma/v2** for typed handlers + OpenAPI 3.1 generation + Stoplight Elements UI
- **Wrangler** for local D1 emulation and prod data export
- **Custom `migrations` package** for versioned schema, run via the `migrate` subcommand

## Quick start

```sh
make dev    # export prod D1 + start the dev server
make watch  # alt: hot reload on .go changes
```

Open <http://localhost:8080/docs> for the API UI.

## Development workflow

```sh
make help             # show all targets
make dev-db           # one-time: re-export prod D1 into .data/dev.sqlite
make run              # start the dev server (fast, ~1s)
make stop             # kill any running dev server (frees port :8080)
make dev              # dev-db + run (first time / data refresh)
make watch            # hot reload on .go changes (requires `go install github.com/air-verse/air@latest`)
make migrate-up       # apply pending migrations
make migrate-down     # roll back the most recent migration
make migrate-status   # show applied and pending migrations
make build            # go build ./...
make test             # go test ./...
make clean            # rm -rf .data .wrangler
```

`go run .` works without flags: `APP_ENV` defaults to `production` only when `CLOUDFLARE_API_TOKEN` is set, otherwise `development`. `LOCAL_DB_PATH` defaults to `.data/dev.sqlite`.

## Configuration

Read from `.env` (shared defaults) and `.env.local` (personal overrides, gitignored). Common keys:

| Key | Default | Notes |
|---|---|---|
| `APP_ENV` | `production` if `CLOUDFLARE_API_TOKEN` set, else `development` | |
| `LOCAL_DB_PATH` | `.data/dev.sqlite` | Used when `APP_ENV=development` |
| `PORT` | `8080` | |
| `CLOUDFLARE_API_TOKEN` | None | D1 edit permission. Required for `production`. |
| `CLOUDFLARE_ACCOUNT_ID` | None | Required for `production`. |
| `CLOUDFLARE_DATABASE_ID` | `735027ae-2327-4561-8e62-538973817b06` | The `krcrackers-products` database. |

Copy `.env.example` to `.env` and fill in the Cloudflare values to talk to prod.

## Migrations

Schema is versioned and lives in `migrations/`. Each file is named `NNNN_name.sql` and uses goose-style `-- +goose Up` / `-- +goose Down` annotations to separate forward and rollback statements:

```sql
-- +goose Up
CREATE TABLE foo (...);

-- +goose Down
DROP TABLE foo;
```

The `migrations` package is a small embedded runner. It tracks applied versions in a `schema_migrations` table and applies statements one at a time through the shared `database.DB` interface — so the same runner works for both the local SQLite database and the remote D1 instance, and there's no duplicate schema definition in service code.

The server no longer auto-migrates on boot. Run migrations explicitly:

```sh
go run . migrate up        # apply pending
go run . migrate down      # roll back the most recent
go run . migrate status    # show applied and pending
```

Or via Makefile: `make migrate-up`, `make migrate-down`, `make migrate-status`. The same commands target dev (SQLite) or prod (D1) — just set `APP_ENV` and the `CLOUDFLARE_*` env vars.

To add a new migration: drop a `NNNN_name.sql` file with a higher version number than the current max. Files are applied in ascending order; `migrate up` only runs what's pending.

## API

All endpoints under `/products` use JSON. Errors follow [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) (`application/problem+json`).

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/products` | `ProductInput` | `201` + `Product` |
| `GET` | `/products` | None | `200` + `[]Product` |
| `GET` | `/products/{id}` | None | `200` + `Product`, `404` if missing |
| `PUT` | `/products/{id}` | `ProductInput` | `200` + `Product`, `404` if missing |
| `DELETE` | `/products/{id}` | None | `204`, `404` if missing |
| `GET` | `/health` | None | `204` |
| `GET` | `/openapi.json` | None | OpenAPI 3.1 spec |
| `GET` | `/docs` | None | Stoplight Elements UI |

### Schema

`ProductInput` (request body): `name`, `price`, `category`, `comparePrice` are required; `brand`, `description`, `image` are optional and nullable.

```json
{
  "name": "Sneaker",          // required, minLength 1
  "price": 99,                // required, >= 0
  "category": "footwear",     // required, minLength 1
  "comparePrice": 129,        // required, >= 0
  "brand": "Acme",            // optional, nullable
  "description": "Shoes",     // optional, nullable
  "image": "/s.png"           // optional, nullable
}
```

`Product` (response) adds the server-assigned `id`.

### Validation

Huma validates path params, body, and required fields automatically and returns `422` with a per-field error list:

```json
{
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "validation failed",
  "errors": [
    {"location": "body.price", "message": "expected number >= 0", "value": -5}
  ]
}
```

## Database

Production: Cloudflare D1, `krcrackers-products` (`735027ae-2327-4561-8e62-538973817b06`), region APAC.

Schema (matches prod exactly: all fields except `id` are nullable):

```sql
CREATE TABLE products (
    id            INTEGER PRIMARY KEY,
    name          TEXT,
    price         REAL,
    brand         TEXT,
    description   TEXT,
    category      TEXT,
    image         TEXT,
    compare_price REAL
);
```

Migrations are managed by the `migrations/` package — see the [Migrations](#migrations) section above. Applied versions are tracked in a `schema_migrations` table that the runner creates automatically.

12 categories in the current data: Bombs, Chakras, Crackers, Fancy, Flower Pots, Gift Boxes, Lar, Laxmi & Kuruvi, Night Crackers, Rocket, Shots, Sparkles.

## Project structure

```
.
├── main.go                    # server entry, huma setup, lifecycle, migrate subcommand
├── config/config.go           # env loading (godotenv, .env.local override)
├── database/
│   ├── database.go            # DB interface + factory (mode → D1 or SQLite)
│   ├── d1.go                  # Cloudflare D1 backend
│   └── sqlite.go              # local SQLite backend (modernc.org/sqlite)
├── dbconv/dbconv.go           # any → primitive coercion for DB rows
├── services/
│   ├── orders/                # placeholder
│   └── products/main.go       # huma operations + DB access
├── migrations/
│   ├── migrations.go          # embedded SQL runner, version tracking
│   └── 0001_init.sql          # goose-format schema (Up / Down)
├── wrangler.toml              # D1 binding
├── Makefile                   # dev-db / run / dev / watch / migrate-*
├── .air.toml                  # hot reload config
└── .env.example               # config template
```

The `database.DB` interface is backend-agnostic. Adding another backend (Postgres, in-memory for tests, etc.) is one new file that implements `Query` / `Execute` / `Close` and a switch case in `database.New`.

## Deployment

Migrations and server start are separate steps. Apply migrations as a pre-deploy step (CI job, init container, or manual run) before starting the server:

```sh
# 1. Apply migrations
APP_ENV=production CLOUDFLARE_API_TOKEN=... \
  CLOUDFLARE_ACCOUNT_ID=... CLOUDFLARE_DATABASE_ID=... \
  ./app migrate up

# 2. Start the server
APP_ENV=production CLOUDFLARE_API_TOKEN=... \
  CLOUDFLARE_ACCOUNT_ID=... \
  CLOUDFLARE_DATABASE_ID=735027ae-2327-4561-8e62-538973817b06 \
  ./app
```

The Go binary is platform-agnostic. In `production` mode it talks to D1 over HTTPS using the cloudflare-go SDK. No edge runtime needed, just a regular Linux host.

If you ever need to deploy a Worker alongside, the `wrangler.toml` is already set up with the D1 binding. Just point a `[[d1_databases]]` entry at a Worker and reuse `migrations/`.
