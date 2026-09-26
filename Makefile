DB_NAME    := krcrackers-products
DUMP_FILE  := .data/prod.sql
DB_FILE    := .data/dev.sqlite
PORT       ?= 8080
AIR        := $(shell command -v air 2>/dev/null || echo "$$(go env GOPATH 2>/dev/null)/bin/air")
SWAG       := $(shell command -v swag 2>/dev/null || echo "$$(go env GOPATH 2>/dev/null)/bin/swag")
ENV_FILE   := .env.production

# Frees $(PORT) without matching process names. `pkill -f krcracker` also matches
# the sibling frontend, whose path (krcrackers-fe) contains the same substring.
FREE_PORT = @pids="$$(lsof -ti:$(PORT) 2>/dev/null)"; \
	if [ -n "$$pids" ]; then kill $$pids 2>/dev/null || true; sleep 1; fi; \
	pids="$$(lsof -ti:$(PORT) 2>/dev/null)"; \
	if [ -n "$$pids" ]; then kill -9 $$pids 2>/dev/null || true; sleep 1; fi; \
	echo "port $(PORT) free"

.DEFAULT_GOAL := help

.PHONY: help dev-db dump run dev stop watch migrate-up migrate-down migrate-status build build-lambda deploy-lambda deploy-env test test-endpoints bench load clean wrangler-login docs docs-update

help:                ## Show this help message
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

dev-db:              ## Re-export prod D1 into .data/dev.sqlite (requires CLOUDFLARE_* in .env)
	@mkdir -p .data
	@# Export first: the previous ordering deleted the local database before the
	@# export ran, so any export failure destroyed the working local copy.
	go run ./src dump $(DUMP_FILE)
	@rm -f $(DB_FILE) $(DB_FILE)-shm $(DB_FILE)-wal
	sqlite3 $(DB_FILE) < $(DUMP_FILE)
	@# ./src, not .: the module root holds no Go files.
	@go run ./src migrate up
	@echo "Imported $(DB_NAME) -> $(DB_FILE): $$(sqlite3 $(DB_FILE) "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'") table(s), $$(sqlite3 $(DB_FILE) 'SELECT COUNT(*) FROM products') products, $$(sqlite3 $(DB_FILE) 'SELECT COUNT(*) FROM users') users, $$(sqlite3 $(DB_FILE) 'SELECT COUNT(*) FROM orders') orders"

dump:               ## Export prod D1 to .data/prod.sql without touching the local database
	@mkdir -p .data
	go run ./src dump $(DUMP_FILE)

run:                 ## Start the dev server (uses .env / .env.local if present)
	go run ./src

dev: dev-db run      ## First-time / data-refresh: re-export then start

stop:                ## Kill whatever holds :$(PORT) (frees the port)
	$(FREE_PORT)

watch: dev-db        ## Hot reload on .go changes (requires `go install github.com/air-verse/air@latest`)
	@if [ ! -x "$(AIR)" ]; then \
		echo "air not found. Install: go install github.com/air-verse/air@latest"; \
		exit 1; \
	fi
	$(AIR)

migrate-up:          ## Apply pending migrations to the configured database (dev or prod)
	go run ./src migrate up

migrate-down:        ## Roll back the most recent migration
	go run ./src migrate down

migrate-status:      ## Show applied and pending migrations
	go run ./src migrate status

build:               ## Compile all packages
	go build ./...

build-lambda:        ## Build binary for AWS Lambda (linux/arm64)
	GOOS=linux GOARCH=arm64 go build -o bootstrap ./src/cmd/lambda
	zip lambda.zip bootstrap

deploy-lambda: build-lambda deploy-env  ## Deploy to AWS Lambda (code + env)
	aws lambda update-function-code \
		--function-name krcrackers \
		--region ap-south-1 \
		--zip-file fileb://lambda.zip

deploy-env:              ## Push .env.production vars to Lambda config
	@if [ ! -f $(ENV_FILE) ]; then echo "ERROR: $(ENV_FILE) not found"; exit 1; fi
	@ENV_JSON=$$(python3 -c "import json; d=dict(l.split('=',1) for l in open('$(ENV_FILE)').read().strip().splitlines() if l and not l.startswith('#') and '=' in l); print(json.dumps({'FunctionName': 'krcrackers', 'Environment': {'Variables': d}}))"); \
	aws lambda update-function-configuration \
		--function-name krcrackers \
		--region ap-south-1 \
		--cli-input-json "$$ENV_JSON"

test:                ## Run tests
	go test ./...

test-endpoints:      ## Run endpoint integration tests (starts server, tests all APIs, cleans up)
	./scripts/test-endpoints.sh

bench:               ## Run benchmarks (database, eventbus, products)
	go test ./tests/database/ ./tests/eventbus/ ./tests/services/products/ -run=NONE -bench=. -benchmem

load:                ## Load test read paths with k6 (requires k6; server must be running)
	k6 run -e BASE_URL=$${BASE_URL:-http://localhost:8080} scripts/load.js

clean:               ## Remove .data/ and .wrangler/
	rm -rf .data .wrangler

wrangler-login:      ## Authenticate wrangler with Cloudflare
	wrangler login

docs:                ## Build and serve docs site
	cd docs && bun install && bun run build && bun run start

docs-update:         ## Regenerate OpenAPI spec, build, and serve docs
	$(SWAG) init --output ./docs/openapi --outputTypes json --parseInternal
	npx swagger2openapi docs/openapi/swagger.json -o docs/openapi/openapi.json 2>/dev/null
	cd docs && bun install && bun run build && bun run start
