# External Integrations

## Integration Inventory

| System | Type | Purpose | Auth model | Criticality |
|--------|------|---------|------------|-------------|
| Cloudflare D1 | Database (HTTP API) | Production database | API token | High |
| Cloudflare R2 | Object storage | Payment screenshots, file uploads | Access key + secret key | Medium |
| Meilisearch | Search engine | Product full-text search with typo-tolerance | API key | Medium |
| WhatsApp Cloud API | Messaging | Order lifecycle notifications | API token | Low |
| Google JWKS | Identity | ID token validation for Google login | Public keys (cached 6h) | High |
| AWS Lambda | Compute | Serverless deployment target (secondary) | IAM role | Low |

## Data Stores

| Store | Role | Access layer | Key risk |
|-------|------|--------------|----------|
| Cloudflare D1 | Production database | `database/d1.go` (HTTP API) | Best-effort transactions not atomic |
| Local SQLite | Development database | `database/sqlite.go` (pure-Go) | WAL mode; foreign keys enabled |
| Cloudflare R2 | File storage | `services/uploads/main.go` (S3 API) | No retry logic |
| Meilisearch | Search index | `services/search/main.go` | No retry logic |

## Secrets and Credentials

All credentials are loaded from environment variables via `godotenv`:

| Variable | Purpose | Required |
|----------|---------|----------|
| `JWT_SECRET` | HMAC signing key for JWT tokens | Yes (panics if empty) |
| `CLOUDFLARE_API_TOKEN` | D1 API token (production) | Production only |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account ID | Production only |
| `CLOUDFLARE_DATABASE_ID` | D1 database ID | Production only |
| `R2_ACCESS_KEY_ID` | R2 access key | For file uploads |
| `R2_SECRET_ACCESS_KEY` | R2 secret key | For file uploads |
| `R2_BUCKET_NAME` | R2 bucket name | For file uploads |
| `MEILISEARCH_API_KEY` | Meilisearch API key | For search |
| `WHATSAPP_API_TOKEN` | WhatsApp Cloud API token | For notifications |

No hardcoded secrets found. `.env.local` is gitignored for personal overrides.

## Reliability

- **Timeouts**: HTTP server `ReadHeaderTimeout: 10s`, JWKS fetch `10s`, WhatsApp API `10s`
- **Retry/backoff**: None implemented for external calls
- **Circuit breaker**: None
- **Graceful shutdown**: 10s timeout on SIGTERM/SIGINT
