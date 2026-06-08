# Coding Conventions

## Core Sections (Required)

### 1) Naming Rules

| Item | Rule | Example | Evidence |
|------|------|---------|----------|
| Files | Lowercase, short, descriptive | `database.go`, `sqlite.go`, `repository.go` | `database/`, `services/` |
| Packages | Lowercase singular | `products`, `orders`, `database`, `server` | All package declarations |
| Exported functions | PascalCase | `NewService`, `NewRepository`, `WriteJSON` | `services/products/main.go:42` |
| Unexported functions | camelCase | `rowToProduct`, `validateProductInput`, `paramsToStrings` | `services/products/repository.go:131` |
| Struct types | PascalCase | `Product`, `OrderInput`, `ListProductsResponse` | `services/products/main.go:26` |
| Struct fields (exported) | PascalCase with JSON tags | `ComparePrice float64 \`json:"comparePrice"\`` | `services/products/main.go:23` |
| Interface types | PascalCase, minimal | `DB`, `Tx`, `Row`, `Repository` | `database/database.go:36`, `services/products/repository.go:10` |
| Constants | camelCase (unexported) or PascalCase (exported) | `versionTable`, `ModeLocal`, `ModeD1` | `migrations/migrations.go:44`, `database/database.go:56-58` |

### 2) Formatting and Linting

- Formatter: Standard `gofmt` (Go default)
- Linter: [TODO] — No `golangci-lint`, `.golangci.yml`, or linter config detected
- Most relevant enforced rules: None configured; relies on `gofmt` for formatting
- Run commands: `make build` (compiles all packages), `make test` (runs `go test ./...`)

### 3) Import and Module Conventions

- Import grouping/order: Standard library first, then external packages, then internal packages (implicit Go convention)
- Alias vs relative import policy: No aliases; all imports use package paths directly (e.g., `github.com/thaletto/krcrackers-go/database`)
- Public exports/barrel policy: No barrel exports; each package exports only what's needed

### 4) Error and Logging Conventions

- Error strategy by layer:
  - **Handlers**: Call `server.WriteError(w, status, msg)` with generic messages. No internal errors exposed.
  - **Repositories**: Return `fmt.Errorf("context: %w", err)` with wrapped errors
  - **Database adapters**: Return `TypeError` struct on type mismatches, `fmt.Errorf` for connection/query errors
  - **Config**: Return `fmt.Errorf("context: %w", err)`
- Logging style: `log.Printf` with minimal context (migration version, server mode, request method/path/status/duration)
- Sensitive-data redaction rules: [TODO] — No explicit redaction logic found; Cloudflare tokens are in env vars only

### 5) Testing Conventions

- Test file naming/location rule: [TODO] — No test files exist yet
- Mocking strategy norm: [TODO] — No test infrastructure exists
- Coverage expectation: [TODO] — No coverage threshold configured

### 6) Evidence

- `services/products/main.go` — representative handler with naming patterns
- `services/products/repository.go` — representative repository with error wrapping
- `server/server.go` — `WriteJSON`/`WriteError` error handling helpers
- `database/sqlite.go` — `TypeError` custom error type for type mismatches
- `main.go:96` — `log.Printf` for server startup logging

## Extended Sections (Optional)

Not needed for this small project.
