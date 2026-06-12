# ADR-001: Meilisearch over SQLite FTS5 for Product Search

## Status

Accepted

## Context

The product catalog needs full-text search with filtering, sorting, and
typo-tolerance. The target market is India, where users frequently misspell
product names (e.g., "samsng", "iphne", "lapotp").

Two options evaluated:

- **SQLite FTS5**: built-in, zero infrastructure, but no typo-tolerance
- **Meilisearch**: lightweight search engine, self-hosted or Docker, typo-tolerant

## Decision

Use Meilisearch for product search.

## Consequences

- Adds a Docker container (or managed service) to the stack
- Products are indexed in Meilisearch on create/update/delete
- Provides typo-tolerance, faceted filtering, and instant search
- No vendor lock-in (self-hosted, MIT licensed)
- FTS5 remains available for other internal queries if needed
