# Configuration

Read from `.env` (shared defaults) and `.env.local` (personal overrides, gitignored).

| Key | Default | Notes |
|---|---|---|
| `APP_ENV` | `production` if `CLOUDFLARE_API_TOKEN` set, else `development` | |
| `LOCAL_DB_PATH` | `.data/dev.sqlite` | Used when `APP_ENV=development` |
| `PORT` | `8080` | |
| `CLOUDFLARE_API_TOKEN` | None | D1 edit permission. Required for `production`. |
| `CLOUDFLARE_ACCOUNT_ID` | None | Required for `production`. |
| `CLOUDFLARE_DATABASE_ID` | `735027ae-2327-4561-8e62-538973817b06` | The `krcrackers-products` database. |

Copy `.env.example` to `.env` and fill in the Cloudflare values to talk to prod.

`go run .` works without flags: `APP_ENV` defaults to `production` only when `CLOUDFLARE_API_TOKEN` is set, otherwise `development`.
