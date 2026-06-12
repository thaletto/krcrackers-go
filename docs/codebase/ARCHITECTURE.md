# Architecture

## Architectural Style

- **Primary**: Layered (Service → Repository → Database interface) with event-driven pub/sub
- **Constraints**: Dual backend (SQLite local, D1 prod), no ORM, Go stdlib `net/http` with Go 1.22+ routing

## System Flow

```text
HTTP Request
  → main.go (mux routing via Go 1.22+ method/path patterns)
    → auth middleware (validates JWT, injects user into context)
      → service handler (decode JSON, validate, call repository)
        → repository (build SQL, call database.DB.Query/Execute or Tx)
          → database adapter (SQLite or D1 HTTP API)
            → response rows mapped via database.Row typed accessors
          ← structured error or domain struct
        ← service handler writes JSON via server.WriteJSON
      ← on mutation: publish event to eventbus.Bus
        → subscriber goroutines (search sync, notifications)
      ← HTTP Response
```

## Layer Responsibilities

| Layer | Owns | Must not own |
|-------|------|--------------|
| `server` | JSON response helpers, request logging | Business logic, data access |
| Auth middleware | JWT validation, user injection | Business logic |
| Service handlers | HTTP decode/validate/respond, route registration | SQL, row mapping, transactions |
| Repositories | SQL queries, row mapping, transaction orchestration | HTTP handling, validation |
| `database` | `DB`/`Tx`/`Row` interfaces, adapter implementations | SQL queries |
| `eventbus` | Event pub/sub, handler registration | Business logic |
| `config` | Env loading, config struct construction | Business logic |

## Event-Driven Architecture

The event bus decouples services for cross-cutting concerns:

```text
Products Service ──publish──→ eventbus.Bus ──subscribe──→ Search Subscriber (Meilisearch sync)
                                         ──subscribe──→ Notification Subscriber (WhatsApp)
Orders Service ───publish──→ eventbus.Bus ──subscribe──→ Search Subscriber
                                       ──subscribe──→ Notification Subscriber
```

Handlers run asynchronously via `context.Background()` goroutines. Publishers don't wait for subscribers.

## Authentication Flow

```text
Client                    Server                    Google
  │                         │                         │
  ├─ POST /auth/google ────→│                         │
  │   { idToken: "..." }    │                         │
  │                         ├─ GET /oauth2/v3/certs ─→│
  │                         │←─ JWKS public keys ─────┤
  │                         ├─ Verify JWT signature    │
  │                         ├─ Check issuer            │
  │                         ├─ Create/find user        │
  │                         ├─ Generate access token   │
  │                         ├─ Generate refresh token  │
  │                         ├─ Store refresh in DB     │
  │←─ Set-Cookie ───────────┤                         │
  │   access_token (15min)  │                         │
  │   refresh_token (7d)    │                         │
  │                         │                         │
  │─ GET /products ────────→│                         │
  │   Cookie: access_token  │                         │
  │                         ├─ Validate JWT            │
  │                         ├─ Inject user in context  │
  │←─ 200 + products ───────┤                         │
```

## Order Status State Machine

```text
pending ──→ confirmed ──→ shipped ──→ delivered
  │            │            │
  └──→ cancelled ←──→ cancelled ←──→ cancelled
```

Each transition publishes an event for notification delivery.

## Known Risks

- **D1 transactions not atomic**: `d1Tx.Commit()` executes sequentially; crash mid-commit leaves partial state
- **No test files**: Zero `_test.go` files; regressions undetected
- **No linter config**: No automated style enforcement
