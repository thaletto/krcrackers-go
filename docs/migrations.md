# Migrations

Schema is versioned and lives in `migrations/`. Each file is named `NNNN_name.sql` and uses goose-style `-- +goose Up` / `-- +goose Down` annotations:

```sql
-- +goose Up
CREATE TABLE foo (...);

-- +goose Down
DROP TABLE foo;
```

The `migrations` package is a small embedded runner. Version parsing uses `goose.NumericComponent`; section and statement splitting stay local because goose's statement parser is internal and the D1-HTTP runner is the value-add. It tracks applied versions in a `schema_migrations` table and applies statements one at a time through the shared `database.DB` interface — so the same runner works for both the local SQLite database and the remote D1 instance, and there's no duplicate schema definition in service code.

The server does **not** auto-migrate on boot. Run migrations explicitly:

```sh
make migrate-up       # apply pending
make migrate-down     # roll back the most recent
make migrate-status   # show applied and pending
```

Or directly:

```sh
go run . migrate up
go run . migrate down
go run . migrate status
```

The same commands target dev (SQLite) or prod (D1) — just set `APP_ENV` and the `CLOUDFLARE_*` env vars.

To add a new migration: drop a `NNNN_name.sql` file with a higher version number than the current max. Files are applied in ascending order; `migrate up` only runs what's pending.
