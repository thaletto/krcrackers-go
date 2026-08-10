# AGENTS.md

## Build & verify

```sh
make build   # go build ./... - the only real compile check
make test    # go test ./... - always passes (no test files exist yet)
```

Run `make stop` before `make run` if you suspect a prior server is still bound to `:8080`.

## Endpoint integration tests

```sh
make test-endpoints   # starts server, runs 64 endpoint tests, cleans up
```

## Pagination convention

`limit` and `offset` on list endpoints are optional `int` query params. A value of `0` (the zero value) means "omitted" - omitting `limit` returns all rows with no `LIMIT` clause. The response wraps items and echoes back the params as nullable pointers:

```json
{
  "items": [...],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

When the client omits `limit`, `limit` and `offset` are omitted from the response (`omitempty`).

## Authentication

JWT + refresh tokens in HttpOnly cookies. No tokens in response body.

- `access_token` cookie: 15-minute expiry, signed with `JWT_SECRET`
- `refresh_token` cookie: 7-day expiry, stored in `refresh_tokens` table
- Google login: frontend sends ID token via `POST /auth/google`, backend validates via JWKS (cached 6h)

Middleware: `WithAuth` (required), `WithAdmin` (admin role check), `WithOptionalAuth` (attaches user if present).

## Database seam

All data access goes through the `database.DB` interface (`Query`, `Execute`, `Begin`, `Close`). Rows are **not** `*sql.Row` - use the typed `Row` interface: `row.Int("col")`, `row.String("col")`, `row.Float("col")`, `row.NullableString("col")`. Both backends (SQLite, D1) implement this.

Transactions use `database.Tx` (`Query`, `Execute`, `Commit`, `Rollback`). SQLite wraps `*sql.Tx` (real atomicity). D1 buffers statements and executes on commit (best-effort; not truly atomic).

Adding a backend = one new file implementing `DB` + `Tx` + `Row` + a case in `database.New`.

## Dual backend: local SQLite vs Cloudflare D1

Same `database.DB` interface for both. `database.New` switches on `Mode`:

- `ModeLocal` (`modernc.org/sqlite`) - uses `LOCAL_DB_PATH` (default `.data/dev.sqlite`)
- `ModeD1` (Cloudflare D1 HTTP API via `cloudflare-go/v7`) - requires `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_DATABASE_ID`

`APP_ENV` defaults to `"development"` unless `CLOUDFLARE_API_TOKEN` is set.

## Service pattern

Each service has two layers:

**Repository** — owns SQL, row mapping, and transactions:

```go
type Repository interface {
    Create(ctx context.Context, input ProductInput) (Product, error)
    List(ctx context.Context, limit, offset int) (ListProductsResponse, error)
    Get(ctx context.Context, id int) (Product, error)
    Update(ctx context.Context, id int, input ProductInput) (Product, error)
    Delete(ctx context.Context, id int) error
}

func NewRepository(db database.DB) Repository { ... }
```

**Service (handlers)** — thin HTTP adapters, delegate to repository:

```go
type Service struct { repo Repository }

func NewService(repo Repository) *Service { ... }

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /path", s.handler)
}

func (s *Service) handler(w http.ResponseWriter, r *http.Request) { ... }
```

Handler wiring lives in `src/main.go` and `src/cmd/lambda/main.go`. The `server` package provides shared HTTP helpers (`WriteJSON`, `WriteError`, `WithLogging`) used by both entry points and services.

## Event bus

In-memory pub/sub via `eventbus.Bus`. Publishers fire events, subscribers handle them asynchronously (goroutines with `context.Background()`).

Events: `product.created`, `product.updated`, `product.deleted`, `order.placed`, `order.confirmed`, `order.shipped`, `order.delivered`, `order.cancelled`.

Subscribers: notifications (WhatsApp).

## Services

| Service | Package | Auth | Description |
|---------|---------|------|-------------|
| Auth | `src/services/auth/` | Public + middleware | Register, login, Google login, refresh, logout, /me |
| Customers | `src/services/customers/` | WithAuth | Profile CRUD, address CRUD with set-default |
| Products | `src/services/products/` | Public reads, admin writes | CRUD, FTS5 search/filter/sort, event publishing |
| Orders | `src/services/orders/` | Public + auth + admin | Checkout, customer orders, admin management, dashboard |
| Search | *(built into products)* | N/A | SQLite FTS5 full-text search via `products_fts` table |
| Uploads | `src/services/uploads/` | None (internal) | R2 file uploads for payment screenshots |
| Notifications | `src/services/notifications/` | None (internal) | WhatsApp Cloud API via event subscriber |
| Invoices | `src/services/invoices/` | WithAuth | On-demand PDF invoice generation |

## Migrations

The server does **not** auto-migrate on boot. Run explicitly:

```sh
make migrate-up       # apply pending
make migrate-down     # rollback latest
make migrate-status   # show applied/pending
```

New migration: add `src/migrations/NNNN_name.sql` (higher number than current max) with goose-style `-- +goose Up` / `-- +goose Down` sections. The runner embeds `*.sql` via `//go:embed`. Use `-- +goose StatementBegin` / `-- +goose StatementEnd` around statements containing semicolons (e.g. triggers).

## Env files

`.env` (shared defaults) → `.env.local` (personal, gitignored). Loaded by `godotenv` at startup. See `.env.example` for keys.

## No test files

There are zero `_test.go` files. `go test ./...` is a no-op. If you add tests, there is no test framework or fixture setup to worry about - just standard `testing` + `go test`.

**Test location convention:** all `_test.go` files live **outside `src/`**. Put every test under `tests/...`, mirroring the package layout one level deeper. Example: tests for `src/services/auth/` go in `tests/services/auth/`. Use external test packages (`package <name>_test`) so the blackbox boundary stays intact. Never create `_test.go` files anywhere under `src/`.

## Documentation

Use `go doc` to look up types, functions, and packages - it's faster and more reliable than reading source files:

```sh
go doc src/database.DB            # interface
go doc src/database.Row           # typed row accessor
go doc src/services/products.Product # response struct
go doc src/eventbus.Bus           # event bus interface
go doc src/services/auth.WithAuth # auth middleware
```

HTML docs: open `docs/index.html` in a browser.

## Agent skills

### Issue tracker

Issues are tracked in the Linear project `KR Crackers`. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the five canonical triage labels without overrides. See `docs/agents/triage-labels.md`.

### Domain docs

This repository uses a single-context domain documentation layout. See `docs/agents/domain.md`.
