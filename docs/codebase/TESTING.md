# Testing Patterns

## Core Sections (Required)

### 1) Test Stack and Commands

- Primary test framework: Go stdlib `testing` package (no additional frameworks)
- Assertion/mocking tools: None configured
- Commands:

```bash
make test             # go test ./... (no test files exist yet)
go test ./...         # equivalent
go test -cover ./...  # coverage (when tests exist)
```

### 2) Test Layout

- Test file placement pattern: [TODO] — No test files exist
- Naming convention: Go convention would be `*_test.go` co-located with source files (e.g., `repository_test.go` next to `repository.go`)
- Setup files and where they run: None

### 3) Test Scope Matrix

| Scope | Covered? | Typical target | Notes |
|-------|----------|----------------|-------|
| Unit | No | — | No test files exist |
| Integration | No | — | No test files exist |
| E2E | No | — | No test files exist |

### 4) Mocking and Isolation Strategy

- Main mocking approach: [TODO] — No test infrastructure; `database.DB` interface is designed for easy mocking (could inject a mock DB for unit tests)
- Isolation guarantees: N/A
- Common failure mode in tests: N/A

### 5) Coverage and Quality Signals

- Coverage tool + threshold: None configured
- Current reported coverage: 0% (no test files)
- Known gaps/flaky areas: Entire test suite is absent

### 6) Evidence

- `AGENTS.md` — States "always passes (no test files exist yet)"
- `Makefile:59-60` — `test` target runs `go test ./...`
- All `.go` files — No `_test.go` files found anywhere

## Extended Sections (Optional)

Not needed for this small project.
