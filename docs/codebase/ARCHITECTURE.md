# Architecture

## Core Sections (Required)

### 1) Architectural Style

- Primary style: Layered (Service → Repository → Database interface)
- Why this classification: Each service has thin HTTP handlers that delegate to a repository, which owns SQL and transactions. The `database.DB` interface is a clean seam between business logic and data access.
- Primary constraints: Dual backend (SQLite local, D1 prod), no ORM (raw SQL everywhere), Go stdlib `net/http` with Go 1.22+ routing patterns

### 2) System Flow

```text
HTTP Request
  → main.go (mux routing via Go 1.22+ method/path patterns)
    → service handler (decode JSON, validate, call repository)
      → repository (build SQL, call database.DB.Query/Execute or Tx)
        → database adapter (SQLite or D1 HTTP API)
          → response rows mapped via database.Row typed accessors
        ← structured error or domain struct
      ← service handler writes JSON via server.WriteJSON
    ← HTTP Response
```

### 3) Layer/Module Responsibilities

| Layer or module | Owns | Must not own | Evidence |
|-----------------|------|--------------|----------|
| `server` | JSON response helpers, request logging | Business logic, data access | `server/server.go` |
| Service handlers (`services/*/main.go`) | HTTP decode/validate/respond, route registration | SQL, row mapping, transactions | `services/products/main.go` |
| Repositories (`services/*/repository.go`) | SQL queries, row mapping, transaction orchestration | HTTP handling, validation | `services/products/repository.go` |
| `database` | `DB`/`Tx`/`Row` interfaces, adapter implementations, factory | SQL queries (delegated to repositories) | `database/database.go` |
| `config` | Env loading, `Config` struct construction | Business logic | `config/config.go` |
| `migrations` | SQL file parsing, version tracking, up/down/status | Business logic, HTTP handling | `migrations/migrations.go` |

### 4) Reused Patterns

| Pattern | Where found | Why it exists |
|---------|-------------|---------------|
| Repository pattern | `services/products/repository.go`, `services/orders/repository.go` | Separates HTTP from SQL; each domain owns its data access |
| Interface-based DB seam | `database/database.go:36-41` (`DB` interface) | Allows swapping SQLite/D1 without changing business code |
| Typed row accessors | `database.Row` interface (`Int`, `String`, `Float`, `NullableString`) | Prevents silent type mismatches; returns `TypeError` on mismatch |
| Best-effort transaction (D1) | `database/d1.go:84-136` (`d1Tx`) | Buffers statements and executes on commit; cleanup on partial failure |
| Embedded migrations | `migrations/migrations.go:42` (`//go:embed *.sql`) | Migrations ship with the binary; no filesystem dependency at runtime |
| Service-owns-repository | `services/*/main.go` creates repository in `NewService()` | Each domain is self-contained; wiring is explicit |

### 5) Known Architectural Risks

- **D1 transactions are not atomic**: `d1Tx.Commit()` executes statements sequentially; a crash mid-commit leaves partial state. SQLite has real atomicity via `*sql.Tx`.
- **String-based error matching**: Services compare `err.Error() == "product not found"` instead of typed errors. Fragile if wording changes.
- **No error type system**: All domain errors are plain `fmt.Errorf` strings; no sentinel errors or custom error types.

### 6) Evidence

- `main.go:115-131` — handler wiring and mux setup
- `cmd/lambda/main.go:35-51` — Lambda handler wiring (identical pattern)
- `database/database.go:36-51` — `DB` and `Tx` interfaces
- `database/d1.go:84-136` — D1 best-effort transaction adapter
- `database/sqlite.go:83-89` — SQLite real transaction adapter
- `services/products/main.go:46-52` — route registration pattern
- `services/products/repository.go:10-16` — repository interface
- `services/orders/repository.go:26-63` — transaction usage in repository
- `migrations/migrations.go:206-235` — migration runner using `database.DB` interface

## Extended Sections (Optional)

Not needed for this small project.
