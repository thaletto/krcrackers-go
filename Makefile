DB_NAME    := krcrackers-products
DUMP_FILE  := .data/prod.sql
DB_FILE    := .data/dev.sqlite
AIR        := $(shell command -v air 2>/dev/null || echo "$$(go env GOPATH 2>/dev/null)/bin/air")
SWAG       := $(shell command -v swag 2>/dev/null || echo "$$(go env GOPATH 2>/dev/null)/bin/swag")
ENV_FILE   := .env.production

.DEFAULT_GOAL := help

.PHONY: help dev-db run dev stop watch migrate-up migrate-down migrate-status build build-lambda deploy-lambda deploy-env test test-endpoints clean wrangler-login docs docs-update

help:                ## Show this help message
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

dev-db:              ## Re-export prod D1 into .data/dev.sqlite (requires wrangler login)
	@mkdir -p .data
	@rm -f $(DUMP_FILE) $(DB_FILE)
	wrangler d1 export $(DB_NAME) --remote --output=$(DUMP_FILE)
	sqlite3 $(DB_FILE) < $(DUMP_FILE)
	@go run . migrate up
	@echo "Imported $$(sqlite3 $(DB_FILE) "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'" | grep -v sqlite | wc -l | tr -d ' ') table(s) from $(DB_NAME) into $(DB_FILE)"

run:                 ## Start the dev server (uses .env / .env.local if present)
	go run .

dev: dev-db run      ## First-time / data-refresh: re-export then start

stop:                ## Kill the running krcracker server (frees port :8080)
	@pkill -f krcracker && echo "killed" || echo "no krcracker process running"

watch: dev-db        ## Hot reload on .go changes (requires `go install github.com/air-verse/air@latest`)
	@if [ ! -x "$(AIR)" ]; then \
		echo "air not found. Install: go install github.com/air-verse/air@latest"; \
		exit 1; \
	fi
	$(AIR)

migrate-up:          ## Apply pending migrations to the configured database (dev or prod)
	go run . migrate up

migrate-down:        ## Roll back the most recent migration
	go run . migrate down

migrate-status:      ## Show applied and pending migrations
	go run . migrate status

build:               ## Compile all packages
	go build ./...

build-lambda:        ## Build binary for AWS Lambda (linux/arm64)
	GOOS=linux GOARCH=arm64 go build -o bootstrap ./cmd/lambda
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
