# krcrackers-go - deep plan (error-handling + safety + observability + tests + lint)

## 0. Constraints and agreed direction

- Keep layered service: `src/apis/*` thin HTTP, `src/services/*` no `net/http`. No clean/hexagonal/DDD migration.
- Keep `src/` layout (`src/main.go:45`, `src/cmd/lambda`). Root `cmd/`+`internal/` deferred - high churn.
- DI stays manual via `src/main.go:197` `newHandler`. No wire/do/fx.
- `samber/mo`: `Option[T]` only for truly nullable (`src/services/products/service.go:29-33`, `src/database/database.go:31-33`, `src/services/orders/types.go:139-140`). `Result` at boundaries only.
- Modernize deferred: `go.mod:3` go 1.26.3 on toolchain 1.27.1; `src/migrations/migrations.go:94,504` `sort.Slice` -> `slices.SortFunc`; `src/services/auth/google.go:122`, `jwt.go:43` `interface{}` -> `any`; `src/database/d1.go:323,374`, `src/eventbus/eventbus.go:62` C-loops -> `range`.
- Status: `go test ./...` green. No `.golangci.yml`. Only `pprof` observability, gated by `!IsProduction` (`src/main.go:248-250`).

## 1. Phase 1 - error correctness (do first, small PRs)

### 1.1 Unify `ErrNotFound` - `src/services/customers/repository.go:212` vs `src/errors/errors.go:5`

Current:
- `src/errors/errors.go:5`: `var ErrNotFound = errors.New("not found")` - canonical, matched with `errors.Is` in `src/apis/products/handler.go:237`, `src/apis/orders/handler.go:223`, `src/apis/invoices/handler.go:94`, `src/apis/customers/handler.go:66,218`.
- `src/services/customers/repository.go:212`: `var ErrNotFound = fmt.Errorf("not found")` - separate value, no chain, allocates per package init. `errors.Is` still matches it against itself, so no outage today, but two canonical 404s exist and future `==` or message edits diverge.

Change:
- Delete local `ErrNotFound` in customers repository. Import `apperrors "github.com/thaletto/krcrackers-go/src/errors"` and return/wrap that sentinel.
- Check other local sentinels for the same pattern: `src/services/products/service.go:22` `ErrInvalidSort` (correct `errors.New`), `src/services/orders/service.go:25,29` (correct), `src/services/orders/actor.go:43` `ErrNotPermitted` (correct). Only customers repo is wrong.
- Keep messages lowercase, no punctuation (already true).

Tests:
- Extend `tests/errors/errors_test.go` + `tests/services/customers/service_test.go`: 404 mapping via canonical sentinel; rename-message regression (change message, test must still pass via `errors.Is`).
- Verify: `grep -rn "fmt.Errorf(\"not found\")" src` == 0; `go test ./tests/errors/... ./tests/services/customers/...`.

### 1.2 Typed validation errors - kill `err.Error()` switches

Current (verified):
- Products: `src/services/products/service.go:196-210` `validateProductInput` returns `fmt.Errorf("name is required")`, `("price must be >= 0")`, `("category is required")`, `("rating must be between 0 and 5")`. Handler `src/apis/products/handler.go:246-252` `isProductValidation` switches on `err.Error()`. Comment in `handler.go:231-234` already admits these strings are client-visible.
- Orders: `src/services/orders/service.go:375-407` `ValidateOrderInput` returns `fmt.Errorf("<field> is required")` for 9 fields + `ErrNoItems` sentinel for empty basket. Handler `src/apis/orders/handler.go:162-173` reconstructs `field+" is required"` matching. Same fragility the customers code documents in `src/services/customers/service.go:121-126` - rewording silently flips 422 -> 500 via `WriteInternalError`.
- Customers (good example to copy): `src/services/customers/service.go:126` `ErrAddressIncomplete` sentinel + `errors.Is` in handler (`handler.go:149,188`).

Change (follow the customers precedent, do not invent a framework):
- Products: add `var ErrProductInvalid = errors.New(...)` per field or one `ValidationError{Field, Reason}` type with `Unwrap`/sentinel match. Minimal option: 4 sentinels (`ErrProductNameRequired`, `ErrProductPriceInvalid`, `ErrProductCategoryRequired`, `ErrProductRatingInvalid`) returned directly from `validateProductInput`; handler matches with `errors.Is`. Prefer this over a generic type - 4 cases, no new API.
- Orders: same - either per-field sentinels or single `ErrOrderFieldRequired` + `ValidationError{Field}` carrying field name, matched with `errors.As` / Go 1.26 `errors.AsType`. Use `errors.AsType` since `go.mod` is 1.26.3. Keep `ErrNoItems`, `ErrTooManyItems`, `ErrInvalidQuantity` etc. as-is (already sentinels matched in `handler.go:140-147`).
- Keep client messages stable: handler still writes the same 422 body text, but branches on type/sentinel, never on `err.Error()`.
- Also fix `src/services/auth/service.go:120`: `fmt.Errorf("%w: %s", ErrInvalidGoogleToken, err.Error())` flattens the cause to a string (chain partially broken, high-cardinality message). Change to `fmt.Errorf("%w: %w", ...)` with a static prefix or `errors.Join`, and match with `errors.Is(err, ErrInvalidGoogleToken)` (handler already does at `src/apis/auth/handler.go:180`).

Tests:
- Table-driven cases in `tests/services/products/service_test.go` and `tests/services/orders/*_test.go`: each invalid field -> matching sentinel/type; handler test asserts 422 for each; rename-message test (change message text, handler mapping must not change).
- Authoring gate per new test: behavior protected (422 vs 500 mapping), credible regression (reword message), no duplicate owner (service owns contract, handler owns status mapping only).
- Verify: `grep -rn "err.Error() ==" src` == 0; `grep -rn "isProductValidation\|isValidationError" src` == 0 after removal.

### 1.3 Swallowed / ignored errors (verified list, fix each explicitly)

- `src/services/auth/repository.go:259`: `_, _ = r.db.Execute(...)` token-rotation best-effort update. Decide: log at debug via `slog` and continue (rotation is opportunistic), but never `_`. Add comment explaining why failure is non-fatal.
- `src/apis/invoices/handler.go:104`: `_, _ = w.Write(res.PDF)`. `http.ResponseWriter.Write` fails on client disconnect. Log (low-cardinality) or count in error metric; do not silently ignore.
- Supporting instances found during audit - same PR or immediate follow-up, same rule:
  - `src/services/customers/repository.go:259-260`: `createdAt, _ := time.Parse(...)`, `updatedAt, _ := ...` - malformed timestamps become zero time. Return `fmt.Errorf("...: %w", err)` like `src/services/auth/repository.go:255` already does for refresh expiry.
  - `src/services/notifications/main.go:67`: `body, _ := json.Marshal(payload)` - marshal of `map[string]any` cannot realistically fail, but handle explicitly (return + log) to satisfy `errcheck`.
  - `src/services/products/service.go:190`, `src/services/orders/service.go:413`: `_ = s.bus.Publish(...)` - publish is fire-and-forget by design (`Publish` always returns nil today in `src/eventbus/eventbus.go:97`). Keep `_ =` only with a `// publish is best-effort` comment, or make `Publish` return nothing. Do not add retry here.
- Verify: `golangci-lint run --enable-only errcheck ./...` clean on touched packages; `grep -rn ", _ =\|_, _ =" src` reviewed case-by-case.

## 2. Phase 2 - reliability (after Phase 1)

### 2.1 `recover` middleware at HTTP boundary

Current: zero `recover()` in `src/`. `src/services/auth/service.go:60-66` `NewServiceWithGoogleVerifier` panics on empty `JWT_SECRET` - acceptable fail-fast at startup, but any request-time panic (nil deref on `*string`/`*float64` optionals like `src/services/products/service.go:29-33`) crashes the process. `src/main.go:179-183` `ListenAndServe` has no recovery; Lambda entry (`src/cmd/lambda/main.go`) same.

Change:
- Add `server.WithRecovery` (next to `WithLogging`/`WithSecurityHeaders` in `src/server/server.go:89-106`): `defer func(){ if r:=recover(); r!=nil { slog.Error(...stack...); WriteInternalError } }`.
- Wire in `src/main.go:260` chain and Lambda handler. Keep panic for init-time config only; never use panic for request errors.
- Log `panic` value + `debug.Stack()` as structured attrs, stable message `http panic recovered`.

Tests: `tests/server/server_test.go` - handler that panics -> 500, process survives, structured log contains stack attr.

### 2.2 Single handling rule - eventbus double-log

Current: `src/services/notifications/main.go:72,80,86` logs send failures and returns void (fire-and-forget `Service` interface has no error return). `src/eventbus/eventbus.go:70-72,91-93` logs every handler error. If a notification subscriber ever returns errors through the bus, the same failure logs twice (subscriber + bus). Today WhatsApp failures log once (subscriber) and bus logs nothing (nil error), but the structure invites duplication.

Change:
- Keep `notifications.Service` fire-and-forget (changing the interface ripples to `noopService` + all call sites). Rule: leaf logs once with stable message + attrs (`template`, `order_id`, `status`), bus stays the top-level logger for handlers that return errors. Do not log-and-return the same error in both.
- WhatsApp `sendTemplate`: replace 3 `log.Printf` with `slog.Warn/Error` + attrs; include `resp.Body` drain/close already present (`main.go:83` `defer resp.Body.Close()` is correct - not in a loop).
- `eventbus.run`/`Publish` inline path: one `slog.Error("event handler failed", "event", name, "error", err)`.

### 2.3 `log.Printf` -> `slog` (prereq for observability)

Current: ~14 `log.Printf` sites, zero `slog`. `src/server/server.go:99-106` `WithLogging` logs unstructured `"%s %s %d %s"`. `src/server/server.go:57` `WriteInternalError` logs internals (correct place, wrong format).

Change (stdlib only, no zap/logrus):
- `slog.New(slog.NewJSONHandler(os.Stdout, nil))` + `slog.SetDefault` in `src/main.go:runServer` and `src/cmd/lambda/main.go`. Lambda already uses `log.Fatalf` for init failures (`main.go:32,37`) - keep fatal, switch to `slog.Error` + `os.Exit(1)` for structured init logs.
- `WithLogging` -> structured: `slog.InfoContext(ctx, "http request", "method", "route", "status", "duration_ms")`. Use route pattern (`r.Pattern` / registered pattern), never raw `r.URL.Path` or user IDs as grouping keys.
- Keep `WriteInternalError` semantics (log technical, return `"internal error"`), change transport to `slog.Error("internal error", "error", err, "path", route, ...)`. Never put tokens, account IDs, SQL fragments in client bodies - current code already avoids this (`src/apis/orders/handler.go:152-155` comment documents a prior leak).
- Migration order: server + eventbus + notifications first; migrations/CLI (`src/migrations/migrations.go:432,474`, `src/main.go:94,105`) last (human-readable CLI output may stay `fmt`/`log`).

## 3. Safety follow-ups (small, after Phase 1)

Verified clean: comma-ok asserts (`src/database/d1.go:70,259,282,416,436,474,488`), `make(map...)` everywhere (`src/eventbus/eventbus.go:59`, `src/database/d1.go:313,369`, `src/database/sqlite.go:96-97`, `src/migrations/migrations.go:312,491`, `src/adapters/adapters.go:55`), no `defer`-in-loop, no float `==`.

1. `int64 -> int` (`src/services/customers/repository.go:133,262-263`, `src/services/products/repository.go:48`): `int(res.LastInsertID)` truncates on 32-bit. Fix: bounds-check against `math.MaxInt` or change `Address.ID`/`Product.ID` to `int64`. Prefer bounds-check helper in `src/database/` (one place, both repos) - smaller API churn than widening IDs.
2. Nil interface (`src/main.go:202-236`): `var uploadSvc uploads.Service` (nil interface) assigned into `orders.Intake{Uploads: uploadSvc}`. Callee checks `s.intake.Uploads == nil` (`src/services/orders/service.go:230`) - correct because the stored value is a nil interface, not a typed-nil pointer (`NewService` returns `(Service, error)` and the error branch is handled, so no `(*service)(nil)` is stored). Keep this pattern; add a regression test asserting guest checkout without R2 works. Same for `bus may be nil` (`service.go:44`, `publishOrderEvent` nil-checks at `409-412` - correct).
3. No action: `src/database/dump.go:180-181` float->int already bounds-checked (`x < 1e15`); `src/apis/orders/handler.go:362` `defer file.Close()` is single-file scope, not a loop.

## 4. Observability (after slog)

1. Metrics (Prometheus client, not yet vendored): `http_server_duration_seconds` Histogram + `http_server_errors_total` Counter, labels `(method, route, status_class)` only. Every endpoint in `README.md:69-116` gets latency + error rate. Exemplars with `trace_id` when tracing lands. Never label with user IDs, full URLs, order IDs.
2. Tracing (OTEL SDK, new dependency): `TracerProvider` in `main`, `otelhttp` middleware, spans on service methods + `src/database/` queries + R2/WhatsApp/D1 calls, `span.RecordError`. Propagate `ctx` (already threaded through services/repos). Sample in prod; full in dev.
3. Profiling: keep `pprof` dev-only (`src/main.go:248`). Add env-toggle for continuous profiling (Pyroscope) later; never expose `pprof` in prod without auth.
4. Logs-traces-metrics correlation: `otelslog` bridge or manual `trace_id`/`span_id` attrs; metric comments carry PromQL + alert rule drafts.
5. Out of scope: RUM/PostHog/Segment (API backend, no browser). Business events (checkout, invoice) tracked server-side only if product asks.

Cost note: histograms + tracing add CPU/memory and vendor ingest. Start with slog + RED metrics on 4 critical routes (`POST /orders/checkout`, `GET /orders/my`, `GET /products`, `GET /invoices/{id}`), expand after dashboards prove useful.

## 5. Tests (golang-testing + test-audit) - green, blackbox, gaps enumerated

Verified: 20 test files under `tests/` mirroring `src/` domains (`tests/services/orders/service_test.go`, `checkout_test.go`, `write_test.go`, `pricing_test.go` for `src/services/orders/service.go` etc.); all packages `xxx_test` blackbox (e.g. `package orders_test`, `package products_test`) - correct per skill. `tests/services/orders/service_test.go:23` `fakeCatalogue` mocks the consumed interface, not concrete types - correct. No `testify` assert-scope leak possible (no testify in use); no order-dependent tests found; no `Example`/`Fuzz` targets.

Gaps (fix in test PRs alongside Phase 1, not as a campaign):
- File naming: `bench_test.go` (`tests/services/products/bench_test.go`) violates one-file-per-source (`service.go` -> `service_test.go` + `service_bench_test.go` expected) and bench files drift from measured-function order. Rename to `service_bench_test.go` (+ `database/service_bench_test.go`, `eventbus/eventbus_bench_test.go`) when touching benchmarks (§14).
- No `t.Parallel()` anywhere, no `goleak.VerifyTestMain`, no `//go:build integration`, no `t.Context()`, no `synctest`. SQLite-backed service tests (e.g. `productServiceBench` in `bench_test.go:15-31` opens per-bench SQLite + migrates + seeds 100 rows) are really integration tests running unconditionally. Plan: keep running by default (fast, local SQLite), but (a) add `t.Parallel()` to independent subtests, (b) add `goleak.VerifyTestMain` to `tests/eventbus` (goroutine workers), (c) use `t.Context()` (Go 1.24+) instead of `context.Background()` in tests, (d) reserve `synctest` for future eventbus timing tests only.
- Coverage lies: `go test -cover ./tests/...` reports `[no statements]` because code lives in `src/`. Measure with `go test -coverpkg=./src/... ./tests/...` + `go tool cover -html`. Treat % as gap-finder, not target. First coverage reads: validation mappers (§1.2), `recover` middleware (§2.1), `withItems` N+1 path (§15).
- Authoring gate stays: one owner per contract; extend table cases (named `t.Run` subtests, assert per-subtest `t`) instead of new files; no test-only exports. First sweep: string-validation tests move ownership to service sentinels; handler keeps one 422 + one 500.
- Validation per batch: `go test -race` owner + siblings, `git diff --check`, `git diff --numstat` (prod vs test LOC separately).

## 6. Lint (immediately after Phase 1)

- Add `.golangci.yml` from `.agents/skills/golang-lint/assets/.golangci.yml` (48 linters: `govet`, `staticcheck`, `errcheck`, `errorlint`, `nilerr`, `forcetypeassert`, `modernize`, `intrange`, `sloglint`, `gosec`, `bodyclose`, `testifylint`, ...).
- Add `Makefile` targets `lint` / `lint-fix` / `fmt`. Run `golangci-lint run ./...`, `run --fix` for auto-fixables.
- If legacy noise is high, set `issues.new-from-rev` to gate new code first, then clean legacy in parallel lanes (auto-fix / security / error-handling / style / quality).
- `//nolint` only with linter name + justification; never suppress `gosec`/`bodyclose`/`sqlclosecheck` without a recorded reason.

## 7. Code style (golang-code-style) - mostly compliant, 3 fix batches

Verified clean: early returns throughout, receivers consistent per type (`s *Service` in `src/services/orders/service.go:54-409`, `h *Handler` in `src/apis/orders/handler.go:71-599`, `r *repo`, `b *memoryBus`), `ctx` first in all service/repo signatures, composite literals use field names, no `else`-after-`return` found, no dot/blank imports outside `src/main.go:9` `_ "net/http/pprof"` (correct - side-effect import at root only).

Batch A - mechanical (lint-fixable, do with lint rollout):
- 5 lines >120 chars: `src/main.go:204,220`, `src/apis/orders/handler.go:107,368`, `src/database/d1.go:363`. Break at semantic boundaries; 4+ arg calls one-arg-per-line.
- `fmt.Sprintf("%d", ...)` for plain ints: `src/services/notifications/main.go:91-107` (5x orderID), `src/services/invoices/service.go:98`. Use `strconv.Itoa` (`perfsprint` linter). Correct the earlier modernize note: `src/services/auth/google.go:122`, `jwt.go:43` `(interface{}, error)` callbacks MUST stay - signature is fixed by `github.com/golang-jwt/jwt/v5`, changing to `any` breaks the library contract.
- `var` vs `:=`: `src/database/d1.go:231` `var nulls []int`, `src/migrations/migrations.go:79,495`, `src/apis/orders/handler.go:346` nil slices serialize to JSON `null` vs `[]`. Initialize list responses to empty (`[]T{}` / `make`) so `Items` in `src/services/products/service.go:46`, `src/services/orders/types.go:167` never encode `null`. Preallocate with `make(..., 0, n)` only when capacity is known (already done in `src/services/products/repository.go:53-66`, `src/database/sqlite.go:96-97`).

Batch B - judgment (human review, comment if ignored):
- No `Get` renames: `GetProfile`, `GetAddress`, `GetByIDs`, `GetByEmail`, `GetRefreshToken` are repository fetches, not field getters - `Get` prefix is idiomatic here. Only `src/services/auth/context.go:24` `GetUser(r)` could become `UserFromRequest`, low value - skip.
- No boolean renames: `IsDefault` (`src/services/customers/repository.go:22,35`) already has `Is` prefix. No bare `connected`/`permission` found.
- No nesting refactor queued: control flow already guard-clause style; revisit only files flagged by `gocyclo`/`nestif` after lint lands.

Batch C - deferred (breaking churn, do not do now):
- Constructors stutter (`products.NewService`, `orders.NewService`, `auth.NewRepository` - single primary type per package suggests `New`). Renaming all call sites in `src/main.go:212-240` + tests is large-blast-radius for zero behavior gain. Record as deferred; enforce `New` only for new packages.

## 8. Naming (golang-naming) - compliant, 1 correction to modernize note

Verified: MixedCaps throughout, no `snake_case`/`ALL_CAPS`; acronyms correct (`PaymentScreenshotURL` in `src/services/orders/types.go:137`, `HTTP` in handlers); `Err` prefix on sentinels (`ErrNotFound`, `ErrInvalidSort`, `ErrOnlyPendingCancellable`), `Error` suffix on types (`TypeError` in `src/database/database.go:38`); receivers 1-letter and consistent; import aliases only on collision (`authapi`, `apperrors`, `productsSvc` in `src/main.go:17-33` - correct); error strings lowercase, no punctuation; no `util`/`helper`/`common` packages.

Corrections:
- Do NOT apply `any` to jwt callbacks (see §7A). Remove those two lines from the modernize list; the remaining modernize items (`slices.SortFunc`, C-loop -> `range`) stand.
- No enum-zero-value work needed (no iota enums found; `validTransitions` in `src/services/orders/repository.go:461` is a map, `sortOrders` in `src/services/products/service.go:68` is a map).
- Test naming follows `TestXxx` + lowercase subcases per skill; enforce via `revive`/`testifylint` after lint lands, no bulk rename.

## 9. Design patterns (golang-design-patterns) - no new patterns, 2 gaps

Verified good: zero `init()`, `//go:embed *.sql` in `src/migrations/migrations.go:48`, domain pure (`src/services/*` no `net/http`), explicit constructors with injected deps (`src/main.go:212-240`), timeouts on server (`src/main.go:168-176` Read/Write/Idle), HTTP clients (`src/services/auth/google.go:21`, `src/services/notifications/main.go:31` 10s), and CLI ops (`src/main.go:85,137,190` 5/30/10 min). Fail-fast `panic` only for missing `JWT_SECRET` (`src/services/auth/service.go:62`).

Gaps (small, fit inside Phase 2 PRs - no functional-options migration):
- `src/services/auth/google.go:74` `fetchJWKS` uses `httpClient.Get` with no `ctx`: breaks cancel/trace propagation, relies on client timeout only. Change to `NewRequestWithContext(ctx, ...)` + thread `ctx` from `VerifyGoogleIDToken` callers. Same file's double-checked `keyCache` (`google.go:56-72` RWMutex) is correct - keep.
- `src/services/uploads/main.go:35` `LoadDefaultConfig(context.Background())` detaches R2 init from caller. Accept `ctx` param (startup path can pass `context.Background()` explicitly at the single call site in `src/main.go:204`).
- No action: no `New...opts` needed (`orders.NewService` 3 args OK; `uploads.NewService` 5 strings is borderline but an options struct adds churn for one call site); no `var _ Interface = (*Type)(nil)` checks missing on consumer-side interfaces (`InvoiceRenderer` in `src/apis/invoices/handler.go:24` is correctly declared at the consumer); no `math/rand`, no compiled-regexp need, no streaming need (dump pages via `devdb.Export`).

## 10. Concurrency (golang-concurrency) - bounded except 1 overflow path

Verified: `src/eventbus/eventbus.go:51-66` bounded workers (16) + queue (1024); `handlers` map guarded (`Subscribe` Lock in `:100-104`, `Publish` RLock in `:80-82`); `go b.run()` workers exit with process (acceptable for in-memory bus); `src/main.go:179-195` server goroutine + `Shutdown`. `go test -race ./tests/eventbus/...` passes.

Gap:
- `src/eventbus/eventbus.go:86-95` queue-full fallback spawns one unbounded `go func(j job)` per overflow, no `ctx.Done()` select, no `SetLimit`. Under sustained overload this grows without backpressure. Fix options (pick one, small PR): (a) block with `select { case b.queue <- j: case <-ctx.Done(): }` + drop-counter metric, or (b) `errgroup.SetLimit` semaphore. Also add graceful drain on shutdown (today workers never stop, queue never closed) - only if shutdown tests demand it.
- CI: `Makefile:87` `test` runs `go test ./...` without `-race`; add `-race` (and `goleak` in eventbus tests later). No `sync.Map`/`atomic` need; no `time.After`-in-loop; job channel correctly unexported, sender-owned (never closed - correct for process-lifetime bus).

## 11. Context (golang-context) - propagation good, 3 nil/Background cleanups

Verified: `ctx` first everywhere, `r.Context()` threaded handler->service->DB (`src/apis/customers/handler.go:64-248`, `src/database/sqlite.go:38,47,83` `QueryContext`/`ExecContext`), D1 passes `ctx` to Cloudflare SDK (`src/database/d1.go:211`), notifications uses `NewRequestWithContext` (`main.go:70`). No `ctx` stored in structs, no string-key collisions.

Cleanups (fold into Phase 1/2 PRs):
- `src/database/d1.go:166-167` and `src/eventbus/eventbus.go:77-78` `if ctx == nil { ctx = context.Background() }` hide caller bugs; skill prefers `context.TODO()` at the boundary or fixing callers. Change to explicit `TODO` + comment, or drop the guard after verifying callers always pass non-nil (handlers pass `r.Context()`, never nil).
- `src/database/sqlite.go:30` `PingContext(context.Background())` at open + `src/services/uploads/main.go:35` above: acceptable at startup, but mark construction paths as taking `ctx` so future tracing works.
- `fetchJWKS` no-`ctx` (see §9) is the one mid-request break; the rest are entry-point `Background()` uses (`src/main.go:85,137,190`) which are correct.

## 12. Modernize deep (golang-modernize) - corrected shortlist

Toolchain: `go.mod:3` go 1.26.3, runner `go1.27.1`. No `.modernize` file. Decision: stay on 1.26.3 until Phase 2 lands, then bump + `go fix ./...` in an isolated worktree (never on the main tree per skill).

Apply (with lint `modernize`+`intrange` linters):
- `src/migrations/migrations.go:94,504` `sort.Slice` -> `slices.SortFunc`. `src/server/server.go:40` already uses `min` builtin - good.
- `src/eventbus/eventbus.go:62` `for i := 0; i < workers; i++` -> `for range workers` (Go 1.22+). `src/database/d1.go:323,374` C-loops walk string offsets with manual index arithmetic - verify `intrange` accepts the rewrite before touching; skip if the linter stays quiet.
- `t.Context()` / `b.Loop()` in tests, `slices`/`maps`/`cmp.Or` adoption, `json/v2`, stdlib `uuid`: no occurrences justify churn today (`google/uuid` is indirect-only). `slog` migration is already §2.3/§4 - the only high-value modernization.

Do NOT apply: `interface{}` -> `any` on jwt callbacks (library-fixed, see §7A); `os.Root` (no user-supplied file paths - DB path is config, screenshot keys are generated); `math/rand/v2`, deprecated crypto, `GODEBUG` (none found); PGO/tool-directives/monthly CI (defer).

## 13. Benchmarks (golang-benchmark) - 5 benchmarks, modernize to b.Loop

Verified: `tests/database/sqlite_bench_test.go:33` `BenchmarkSQLiteQuery100Rows`, `tests/eventbus/eventbus_bench_test.go:10,20` publish 0/1 subscriber, `tests/services/products/bench_test.go:50,61` list-50 + search. `Makefile:92-96` `bench` runs the 3 packages with `-benchmem`; `load` runs k6 read paths against a live server. All 5 use legacy `b.N` loops (`bench_test.go:54,65`, `eventbus_bench_test.go:15,26`, `sqlite_bench_test.go:37`).

Change (small, with §5 rename):
- Rename bench files to `<source>_bench_test.go`, order `Benchmark*` to mirror source order.
- Convert `b.N` -> `b.Loop()` (Go 1.24+, toolchain 1.27.1); keep `b.ResetTimer()` after seed (already in products bench). Add `-count=10` to `Makefile:93`, compare with `benchstat` on the same toolchain (Go 1.27 allocator skews cross-toolchain deltas).
- Products bench seeds 100 rows per invocation (`bench_test.go:15-31`); keep `ResetTimer` boundary, share via `sync.Once` only if bench time dominates. `perf(scope):` commits carry benchstat + `goos/goarch/cpu` for affected benchmarks only.
- CI: `benchdiff`/`cob` gates later (§14); serial measurement only - never concurrent benchmarks on shared CPU.

## 14. Performance (golang-performance) - profile first, 1 known N+1

Rule: no optimization without a profile. External-first (fgprof off-CPU, traces) - D1/R2/WhatsApp likely dominate request time.

Known hotspot (verified, fix after observability pilot):
- `src/services/orders/repository.go:450-459` `withItems`: one `itemsFor` query per order in `List` (N+1). `src/server/server.go:27-32` already warns unceiled `?limit=` hydrates per-order items. Fix order: (a) keep `MaxPageLimit = 100` enforced via `ParsePage`, (b) batch `SELECT ... WHERE order_id IN (...)` + in-memory join when profiles justify it. Add `BenchmarkOrdersList` mirroring products bench; `benchstat` before/after in commit.
- Suspects, measure only: `src/services/products/repository.go:157` per-ID loop (verify single `IN` query via profile); `src/database/d1.go:73` per-row `json.Marshal` + `paramsToStrings` (D1 path only); per-response `json.NewEncoder` (`server.go:80`) is fine.
- Not doing: custom `http.Transport` (low upstream fan-out), `GOMEMLIMIT`/PGO (no OOM evidence), `sync.Pool`/`unsafe` (nothing in hot paths), `slog.LogAttrs` (only if logging shows in CPU profile).

## 15. Observability definition of done (extends §4)

Per-feature gate: (a) Histogram latency + error Counter with PromQL/alert comments above each var, (b) `slog` context-variant logs, no PII, single-handling, (c) spans on service/DB/external calls with `span.RecordError`, (d) Grafana + alerts wired (seed infra rules from `awesome-prometheus-alerts`), (e) server-side business events only (no browser RUM). Pilot 4 routes first (§4 cost note), expand on signal.

## 16. Database (golang-database) - compliant, 2 justified exceptions

Verified: no ORM (custom `database.DB` seam - sqlx/pgx can't drive D1's HTTP API); parameterized `?` everywhere (`src/services/customers/repository.go:116`, `src/services/products/repository.go:141`, `src/services/orders/repository.go:158`); dynamic identifiers allowlisted (`sortOrders` in `src/services/products/service.go:68`, `scope.clause()`); `ctx` via `QueryContext`/`ExecContext` (`src/database/sqlite.go:38,47,83`) and D1 SDK (`src/database/d1.go:211`); `rows.Close` immediate + `rows.Err` (`src/database/sqlite.go:42,87,115`), repos get materialized `[]Row`; writes `Execute`, reads `Query`; multi-statement txns with rollback (`src/services/customers/repository.go:111`, `src/migrations/migrations.go:360`); D1 non-atomicity documented (`src/database/d1.go:122`, `src/database/database.go:53`, `Capabilities`); nullable via pointers + `NullableString/Float` (`src/database/database.go:31`).

Low priority: (a) no `sql.ErrNoRows` by design - translate `len(rows)==0` to `ErrNotFound` explicitly at each `Get` instead of `ID==0` checks; (b) no pool tuning - fine for single-process SQLite + per-call D1, add past eventbus-scale concurrency; (c) no `FOR UPDATE`/isolation (`BeginTx(ctx, nil)`) - accept while status transitions are admin-serialized (`src/services/orders/repository.go:263` race window noted). Record, don't fix: hand-rolled migration runner (external tools can't drive D1; SQL human-written, versioned, embedded) and FTS triggers (`0001_init.sql:50-62`). Batching = the `withItems` N+1 (§14) via hand-rolled `IN` clause (no sqlx vendored).

## 17. gopls (golang-gopls) - wire once, use for rename-heavy items

Status: binary at `~/go/bin/gopls`, not on `PATH`; MCP/native-LSP/CLI all unwired. Single-module workspace, no `go.work`.

- Wire: `go install golang.org/x/tools/gopls@latest` (v0.20+), add `~/go/bin` to `PATH`; preference MCP (`claude mcp add gopls -- gopls mcp`) → native LSP (`ENABLE_LSP_TOOL=1` + plugin, free post-edit diagnostics) → CLI fallback (`file:line:col`, experimental).
- Session start: `go_workspace` once for layout, then baseline `go_vulncheck` (lightweight; full audit stays `govulncheck` under security).
- Use for §1.1/§1.2 renames (sentinel unify, validation sentinels, `Sprintf`→`Itoa`, `sort.Slice`): `go_symbol_references` blast-radius before each edit (catches interface call sites like `InvoiceRenderer`, `UploadsService` that call-hierarchy misses), `go_diagnostics` mandatory after each change, `go test <changed-packages>` (not `./...`), `go_vulncheck` only if `go.mod` changed. References are build-scoped - re-run for Lambda vs server tags if matches look thin.
- `godig` (pkg.go.dev), not gopls, for versions/licenses/CVEs of packages not yet in the build.

## 18. Documentation (golang-documentation) - application, gaps enumerated

Project type is application (has `main`, produces server + Lambda binaries), so playground demos and `ExampleXxx` are out of scope per the checklist. README exists with quick start, layout, API surface, and docs-site links - keep. OpenAPI auto-generates from swag annotations via `make docs-update` (`docs/openapi/`) - keep annotating new handlers.

Verified present: package comments in `src/config`, `src/eventbus`, `src/adapters`; doc comments on most service/handler code with intent (e.g. `src/services/customers/service.go:121-126` sentinel rationale, `src/apis/orders/handler.go:152-155` leak note).

Gaps (one docs PR, after Phase 2 so rewritten code is documented once):
- Package comments missing (skill: MUST exist): `src/server`, `src/errors`, `src/database`, plus any others found by `grep -L "^// Package"`. One line each with intent, not paraphrase.
- Undocumented exports: `WriteJSON`, `WriteError`, `WithLogging` (`src/server/server.go:77,83,99`), `R2Config` (`src/config/config.go:26`), and whatever `revive`/`godot` flag after lint lands. Intent-over-paraphrase: why the cap/exclusion exists, not what the signature says.
- Missing root files: no `LICENSE`, `CONTRIBUTING.md`, `CHANGELOG.md`, `llms.txt`. Add `LICENSE` (pick one - required even for private repos), minimal `CONTRIBUTING.md` (prereqs, `make dev`, `make test`, PR flow - setup already <10 min via Makefile), `llms.txt` from skill template (helps AI consumers of this repo). Changelog: prefer GitHub Releases over a maintained file while velocity is high; switch to Keep-a-Changelog if release notes start drifting.
- Verify: `revive` exported-rule + `godot` clean, `go doc ./src/...` renders sensibly, `make docs-update` still generates.

## 19. Execution order and verify

Order: wire §17 first (unblocks safe renames) -> 1.1 sentinel -> 1.2 validation types -> 1.3 swallowed errors -> lint config + §7A mechanical style + §12 sort/range -> 2.1 recover -> 2.2 single-log -> 2.3 slog (§9 ctx threading rides along) -> §3 safety IDs + §7B/§8 holds -> §5 test gaps (parallel, goleak, coverpkg, bench renames + §13 b.Loop) -> §10 bus overflow + `-race` -> §4/§15 metrics/tracing pilot + §13 benchstat baseline -> §14 N+1 only if profiled (+ §16 IN-batch) -> §18 docs last (documents final code). Land one coherent PR per item; rerun read-only discovery between batches.

Verify each: `go build ./...`, `go test ./...`, `go test -race ./tests/eventbus/...`, `go test -coverpkg=./src/... ./tests/...`, `golangci-lint run ./...` (once added), `go_diagnostics` clean on touched files, plus `grep -rn "err.Error() ==" src` == 0, `grep -rn "fmt.Errorf(\"not found\")" src` == 0, `awk 'length>120'` on touched files == 0, `grep -rn "b\.N" tests` == 0 after §13, `grep -rn "httpClient.Get\|context.Background()" src` reviewed, no `log.Printf` in touched packages, manual 404/422/`/health`/panic-recovery checks.

Out of scope: `src/` -> `cmd/`+`internal/` move, constructor `NewService` -> `New` rename, `samber/oops` adoption, `errors.Join` rollout, full OTEL/RUM, PGO/GOMEMLIMIT, monthly modernization CI, playground/`ExampleXxx` (application, not library) - revisit after Phase 2 lands.
