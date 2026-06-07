# Deployment

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
