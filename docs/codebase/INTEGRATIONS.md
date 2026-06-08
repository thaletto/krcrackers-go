# External Integrations

## Core Sections (Required)

### 1) Integration Inventory

| System | Type | Purpose | Auth model | Criticality | Evidence |
|--------|------|---------|------------|-------------|----------|
| Cloudflare D1 | Database (HTTP API) | Production database via REST | API token (`CLOUDFLARE_API_TOKEN`) | High | `database/d1.go` |
| AWS Lambda | Compute | Serverless deployment target | IAM role (implicit) | Medium | `cmd/lambda/main.go` |
| AWS API Gateway v2 | API Gateway | HTTP frontend for Lambda | IAM role (implicit) | Medium | `cmd/lambda/main.go:10` |

### 2) Data Stores

| Store | Role | Access layer | Key risk | Evidence |
|-------|------|--------------|----------|----------|
| Cloudflare D1 (`krcrackers-products`) | Production database | `database/d1.go` (HTTP API via `cloudflare-go/v7`) | Best-effort transactions not atomic | `database/d1.go:84-136` |
| Local SQLite (`.data/dev.sqlite`) | Development database | `database/sqlite.go` (pure-Go `modernc.org/sqlite`) | WAL mode; foreign keys enabled | `database/sqlite.go:25` |

### 3) Secrets and Credentials Handling

- Credential sources: Environment variables loaded via `godotenv` from `.env` (shared) and `.env.local` (personal, gitignored)
- Hardcoding checks: No hardcoded secrets found; all credentials read from env vars via `os.Getenv`
- Rotation or lifecycle notes: [TODO] — No rotation mechanism; manual `.env` update

### 4) Reliability and Failure Behavior

- Retry/backoff behavior: [TODO] — No retry logic implemented for D1 HTTP API calls
- Timeout policy: `ReadHeaderTimeout: 10s` on HTTP server (`main.go:93`); migration timeout `5m` (`main.go:53`); Lambda timeout `30s` (`docs/deployment.md`); no D1 HTTP client timeout configured
- Circuit-breaker or fallback behavior: None
- Lambda live endpoint: `https://65bstxj4hottbqu3cg6rm4bx2m0qsxcr.lambda-url.ap-south-1.on.aws` (ap-south-1, arm64, ~2s cold start, <500ms warm)

### 5) Observability for Integrations

- Logging around external calls: Minimal — request logging via `server.WithLogging` (method, path, status, duration) at `server/server.go:25`. D1 API calls have no logging.
- Metrics/tracing coverage: None
- Missing visibility gaps: No D1 API call latency/error logging, no request ID tracking, no structured logging

### 6) Evidence

- `database/d1.go` — D1 HTTP API client implementation
- `database/sqlite.go` — SQLite local adapter
- `cmd/lambda/main.go` — Lambda integration with `go-api-proxy`
- `config/config.go` — credential loading from env vars
- `server/server.go:20-27` — request logging middleware
- `docs/deployment.md` — Lambda deployment details, live endpoint

## Extended Sections (Optional)

Not needed for this small project.
