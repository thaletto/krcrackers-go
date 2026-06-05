# KR Crackers Go Backend

A small Go HTTP API for KR Crackers. Backed by Cloudflare D1 in production, local SQLite in development, with auto-generated OpenAPI docs via [huma](https://github.com/danielgtaylor/huma).

## Stack

- **Go 1.26+**, std `net/http` (Go 1.22+ pattern routing) via `humago` adapter
- **Cloudflare D1** (SQLite at the edge) in production, **local SQLite** via `modernc.org/sqlite` in dev
- **huma/v2** for typed handlers + OpenAPI 3.1 generation + Stoplight Elements UI
- **Wrangler** for local D1 emulation, prod migrations, and prod data export

## Quick start

```sh
make dev    # export prod D1 + start the dev server
make watch  # alt: hot reload on .go changes
```

Open <http://localhost:8080/docs> for the API UI.

## Development workflow

```sh
make help           # show all targets
make dev-db         # one-time: re-export prod D1 into .data/dev.sqlite
make run            # start the dev server (fast, ~1s)
make dev            # dev-db + run (first time / data refresh)
make watch          # hot reload on .go changes (requires `go install github.com/air-verse/air@latest`)
make build          # go build ./...
make test           # go test ./...
make clean          # rm -rf .data .wrangler
make migrate-prod   # apply migrations/0001_init.sql to remote D1
```

`go run .` works without flags: `APP_ENV` defaults to `production` only when `CLOUDFLARE_API_TOKEN` is set, otherwise `development`. `LOCAL_DB_PATH` defaults to `.data/dev.sqlite`.

## Configuration

Read from `.env` (shared defaults) and `.env.local` (personal overrides, gitignored). Common keys:

| Key | Default | Notes |
|---|---|---|
| `APP_ENV` | `production` if `CLOUDFLARE_API_TOKEN` set, else `development` | |
| `LOCAL_DB_PATH` | `.data/dev.sqlite` | Used when `APP_ENV=development` |
| `PORT` | `8080` | |
| `CLOUDFLARE_API_TOKEN` | — | D1 edit permission. Required for `production`. |
| `CLOUDFLARE_ACCOUNT_ID` | — | Required for `production`. |
| `CLOUDFLARE_DATABASE_ID` | `735027ae-2327-4561-8e62-538973817b06` | The `krcrackers-products` database. |

Copy `.env.example` to `.env` and fill in the Cloudflare values to talk to prod.

## API

All endpoints under `/products` use JSON. Errors follow [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) (`application/problem+json`).

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/products` | `ProductInput` | `201` + `Product` |
| `GET` | `/products` | — | `200` + `[]Product` |
| `GET` | `/products/{id}` | — | `200` + `Product`, `404` if missing |
| `PUT` | `/products/{id}` | `ProductInput` | `200` + `Product`, `404` if missing |
| `DELETE` | `/products/{id}` | — | `204`, `404` if missing |
| `GET` | `/health` | — | `204` |
| `GET` | `/openapi.json` | — | OpenAPI 3.1 spec |
| `GET` | `/docs` | — | Stoplight Elements UI |

### Schema

`ProductInput` (request body) — `name`, `price`, `category`, `comparePrice` are required; `brand`, `description`, `image` are optional and nullable.

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

Schema (matches prod exactly — all fields except `id` are nullable):

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

12 categories in the current data: Bombs, Chakras, Crackers, Fancy, Flower Pots, Gift Boxes, Lar, Laxmi & Kuruvi, Night Crackers, Rocket, Shots, Sparkles.

## Project structure

```
.
├── main.go                    # server entry, huma setup, lifecycle
├── config/config.go           # env loading (godotenv, .env.local override)
├── database/
│   ├── database.go            # DB interface + factory (mode → D1 or SQLite)
│   ├── d1.go                  # Cloudflare D1 backend
│   └── sqlite.go              # local SQLite backend (modernc.org/sqlite)
├── services/
│   ├── orders/                # placeholder
│   └── products/main.go       # huma operations + DB access
├── migrations/0001_init.sql   # schema (matches prod)
├── wrangler.toml              # D1 binding
├── Makefile                   # dev-db / run / dev / watch / migrate-prod
├── .air.toml                  # hot reload config
└── .env.example               # config template
```

The `database.DB` interface is backend-agnostic. Adding another backend (Postgres, in-memory for tests, etc.) is one new file that implements `Query` / `Execute` / `Close` and a switch case in `database.New`.

## Deployment

The Go binary is platform-agnostic. In `production` mode it talks to D1 over HTTPS using the cloudflare-go SDK — no edge runtime needed, just a regular Linux host.

```sh
APP_ENV=production \
CLOUDFLARE_API_TOKEN=... \
CLOUDFLARE_ACCOUNT_ID=... \
CLOUDFLARE_DATABASE_ID=735027ae-2327-4561-8e62-538973817b06 \
  ./app
```

If you ever need to deploy a Worker alongside, the `wrangler.toml` is already set up with the D1 binding — just point a `[[d1_databases]]` entry at a Worker and reuse `migrations/`.
