# ADR-002: Cloudflare R2 for File Storage

## Status

Accepted

## Context

Payment screenshots need persistent object storage. The app already uses
Cloudflare D1, so the Cloudflare ecosystem is already adopted.

Options:

- **Local filesystem**: ephemeral on Lambda, lost on cold start
- **AWS S3**: works, but adds another cloud provider
- **Cloudflare R2**: S3-compatible, no egress fees, native Cloudflare integration

## Decision

Use Cloudflare R2 for payment screenshot storage.

## Consequences

- Tight coupling to Cloudflare ecosystem (D1 + R2)
- No egress fees (R2's key differentiator)
- Go SDK available via cloudflare-go/v7
- Migration away from Cloudflare would require replacing both D1 and R2
- Bucket and API tokens managed via wrangler.toml / env vars
