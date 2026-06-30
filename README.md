# KR Crackers Go Backend

A Go HTTP API for KR Crackers e-commerce. Customer-facing order lifecycle, product catalog with search, payment proof uploads, PDF invoices, and WhatsApp notifications.

## Quick start

```sh
make dev    # export prod D1 + start the dev server
make watch  # alt: hot reload on .go changes
```

## Dev commands

```sh
make help             # show all targets
make dev-db           # one-time: re-export prod D1 into .data/dev.sqlite
make run              # start the dev server (~1s)
make stop             # kill any running dev server (frees :8080)
make dev              # dev-db + run (first time / data refresh)
make watch            # hot reload (requires `go install github.com/air-verse/air@latest`)
make migrate-up       # apply pending migrations
make migrate-down     # roll back the most recent migration
make migrate-status   # show applied and pending migrations
make build            # go build ./...
make test             # go test ./... (blackbox tests under tests/)
make test-endpoints   # run 64 endpoint integration tests
make deploy-lambda    # build + push .env.production + aws lambda update-function-code
make clean            # rm -rf .data .wrangler
```

## Project layout

```
src/
  main.go                    # HTTP server entry point
  cmd/lambda/                # AWS Lambda entry point
  adapters/                  # cross-service adapters (UserProvider, AddressProvider)
  config/                    # env-driven config loading
  database/                  # DB seam (SQLite + D1) and typed Row interface
  errors/                    # shared domain sentinels (ErrNotFound)
  eventbus/                  # in-memory pub/sub + event payload types
  migrations/                # goose-style SQL migrations (//go:embed *.sql)
  server/                    # HTTP helpers (WriteJSON, WriteError, WithLogging)
  apis/                      # thin HTTP adapters (decode -> service -> encode)
    auth/                    # register, login, google, refresh, logout, me + middleware
    customers/               # profile + address CRUD
    products/                # public reads, admin writes
    orders/                  # public + customer + admin routes, multipart checkout
    invoices/                # on-demand PDF generation
  services/                  # business logic (no net/http import)
    auth/ customers/ products/ orders/ invoices/ notifications/ uploads/
tests/                       # blackbox tests (package <name>_test), outside src/
```

## Docs

The docs site source is in `docs/content/docs/`. Run `make docs` to build and serve it locally on port 3000.

- [Setup](docs/content/docs/setup.mdx)
- [Architecture](docs/content/docs/architecture.mdx)
- [Services](docs/content/docs/services.mdx)
- [Database](docs/content/docs/database.mdx)
- [Event System](docs/content/docs/events.mdx)
- [Deployment](docs/content/docs/deployment.mdx)
- [OpenAPI Reference](docs/openapi/openapi.md)

## API surface

Public routes:

```
GET    /health
POST   /auth/register
POST   /auth/login
POST   /auth/google
POST   /auth/refresh
POST   /auth/logout
GET    /products
GET    /products/{id}
POST   /orders
GET    /orders
GET    /orders/{id}
PUT    /orders/{id}
DELETE /orders/{id}
GET    /invoices/{id}
```

Authenticated routes (require `access_token` cookie):

```
GET    /auth/me
GET    /customers/profile
PUT    /customers/profile
GET    /customers/addresses
POST   /customers/addresses
PUT    /customers/addresses/{id}
DELETE /customers/addresses/{id}
PUT    /customers/addresses/{id}/default
POST   /orders/checkout
GET    /orders/my
GET    /orders/my/{id}
DELETE /orders/my/{id}
```

Admin routes (require `role: admin`):

```
POST   /admin/products
PUT    /admin/products/{id}
DELETE /admin/products/{id}
GET    /admin/orders
GET    /admin/orders/{id}
PATCH  /admin/orders/{id}/status
GET    /admin/dashboard
GET    /admin/invoices/{id}
```
