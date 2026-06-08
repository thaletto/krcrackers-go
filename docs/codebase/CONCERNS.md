# Codebase Concerns

## Core Sections (Required)

### 1) Top Risks (Prioritized)

| Severity | Concern | Evidence | Impact | Suggested action |
|----------|---------|----------|--------|------------------|
| High | Zero test coverage | All `.go` files — no `_test.go` found | Regressions undetected; refactoring risky | Add repository-level unit tests with mock DB |
| High | D1 transactions not atomic | `database/d1.go:84-136` (`d1Tx`) | Partial order state on crash mid-commit | Document limitation; consider idempotent retry or compensation |
| Medium | String-based error matching | `services/products/main.go:97`, `services/orders/main.go:116` | Breaks if wording changes; no compile-time safety | Introduce sentinel errors or custom error types |
| Medium | No input sanitization on SQL | All repository files — raw `fmt.Sprintf`-style parameterized queries | Low risk (parameterized), but no validation beyond required fields | Add email format validation, price bounds, etc. |
| Low | No CI/CD pipeline | No `.github/` directory detected | Manual `make build` / `make test` only | [TODO] — add CI if desired |

### 2) Technical Debt

| Debt item | Why it exists | Where | Risk if ignored | Suggested fix |
|-----------|---------------|-------|-----------------|---------------|
| No test files | Tests never added; AGENTS.md confirms | All `.go` files | Any change could introduce regressions silently | Add `*_test.go` files for repositories and handlers |
| String error comparison | Quick implementation; no error type system | `services/products/main.go:97,127,149`, `services/orders/main.go:116,140,165` | Fragile; refactor breaks callers | Define `var ErrNotFound = errors.New("not found")` per domain |
| No linter config | Never configured | Project root | No automated style enforcement | Add `.golangci-lint.yml` with basic rules |
| D1 transaction best-effort | D1 HTTP API lacks true transaction support | `database/d1.go:84-136` | Partial commits on failure | Document limitation; consider retry/compensation for critical paths |

### 3) Security Concerns

| Risk | OWASP category (if applicable) | Evidence | Current mitigation | Gap |
|------|--------------------------------|----------|--------------------|-----|
| No auth on endpoints | A01:2021 Broken Access Control | `main.go:115-131` — all routes open | None | Planned for later; no immediate action |
| SQL injection risk (low) | A03:2021 Injection | All repository files | Parameterized queries (`?` placeholders) | Low risk; parameterized queries are used correctly |

### 4) Performance and Scaling Concerns

| Concern | Evidence | Current symptom | Scaling risk | Suggested improvement |
|---------|----------|-----------------|-------------|----------------------|
| No pagination limit default | `services/products/main.go:76-77` | `limit=0` returns all rows | Large product catalog could return huge responses | Add hardcoded default limit (e.g., 20) when none specified |
| Sequential order item inserts | `services/orders/repository.go:43-56` | Items inserted one-by-one in transaction | Slow for large orders | Batch inserts or use `INSERT ... VALUES (...), (...), ...` |
| No connection pooling | `database/sqlite.go:18-34` | Single `sql.Open` call | Fine for SQLite; D1 has no connection concept | N/A for current architecture |

### 5) Fragile/High-Churn Areas

| Area | Why fragile | Churn signal | Safe change strategy |
|------|-------------|-------------|----------------------|
| `services/orders/repository.go` | Most complex repository (transactions, item management) | Largest file (329 lines); recent commits added transaction layer | Test thoroughly; verify transaction rollback paths |
| `database/d1.go` | Best-effort transaction adapter; complex cleanup logic | Added in recent refactor; subtle partial-failure handling | Test with simulated failures; verify cleanup correctness |
| `main.go` | Handler wiring; migration subcommand | Entry point modified across multiple PRs | Keep handler registration centralized; verify both entry points |

### 6) `[ASK USER]` Questions

1. [ASK USER] Is this API intended to be public-facing, or is it an internal tool? (affects auth/rate-limiting requirements)
2. [ASK USER] Are there plans to add authentication/authorization middleware?
3. [ASK USER] Should the default pagination limit be configurable or hardcoded?
4. [ASK USER] Should CI/CD (e.g., GitHub Actions) be added, or is manual `make build` / `make test` sufficient?

### 7) Evidence

- `services/orders/repository.go` — largest file (329 lines), most complex logic
- `database/d1.go:84-136` — D1 transaction adapter with cleanup
- `main.go` — handler wiring and lifecycle
- `Makefile:59-60` — test command (no-op)
- All `.go` files — zero `_test.go` files found

## Extended Sections (Optional)

Not needed for this small project.
