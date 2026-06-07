# KR Crackers Go Backend

A small Go HTTP API for KR Crackers. Backed by Cloudflare D1 in production, local SQLite in development, with auto-generated OpenAPI docs via [huma](https://github.com/danielgtaylor/huma).

## Quick start

```sh
make dev    # export prod D1 + start the dev server
make watch  # alt: hot reload on .go changes
```

Open <http://localhost:8080/docs> for the API UI.

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
make test             # go test ./...
make clean            # rm -rf .data .wrangler
```

## Docs

- [Configuration](docs/configuration.md)
- [API](docs/api.md)
- [Migrations](docs/migrations.md)
- [Database](docs/database.md)
- [Project structure](docs/project-structure.md)
- [Deployment](docs/deployment.md)
