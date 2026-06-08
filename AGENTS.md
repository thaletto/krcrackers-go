# AGENTS.md

## Build & verify

```sh
make build   # go build ./... - the only real compile check
make test    # go test ./... - always passes (no test files exist yet)
```

Run `make stop` before `make run` if you suspect a prior server is still bound to `:8080`.

## Pagination convention

`limit` and `offset` on `GET /products` are optional `int` query params. A value of `0` (the zero value) means "omitted" - omitting `limit` returns all rows with no `LIMIT` clause. The response wraps items and echoes back the params as nullable pointers:

```json
{
  "items": [...],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

When the client omits `limit`, `limit` and `offset` are omitted from the response (`omitempty`).

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

Register services in `server/handler.go` via `svc.RegisterRoutes(mux)`.

Shared HTTP helpers (`WriteJSON`, `WriteError`) live in `serverutil/respond.go` to avoid import cycles.

## Migrations

The server does **not** auto-migrate on boot. Run explicitly:

```sh
make migrate-up       # apply pending
make migrate-down     # rollback latest
make migrate-status   # show applied/pending
```

New migration: add `migrations/NNNN_name.sql` (higher number than current max) with goose-style `-- +goose Up` / `-- +goose Down` sections. The runner embeds `*.sql` via `//go:embed`.

## Env files

`.env` (shared defaults) → `.env.local` (personal, gitignored). Loaded by `godotenv` at startup. See `.env.example` for keys.

## No test files

There are zero `_test.go` files. `go test ./...` is a no-op. If you add tests, there is no test framework or fixture setup to worry about - just standard `testing` + `go test`.

## Documentation

Use `go doc` to look up types, functions, and packages - it's faster and more reliable than reading source files:

```sh
go doc database.DB            # interface
go doc database.Row           # typed row accessor
go doc products.Product       # response struct
```
