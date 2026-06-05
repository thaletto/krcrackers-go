DB_NAME    := krcrackers-products
DUMP_FILE  := .data/prod.sql
DB_FILE    := .data/dev.sqlite
AIR        := $(shell command -v air 2>/dev/null || echo "$$(go env GOPATH 2>/dev/null)/bin/air")

.DEFAULT_GOAL := help

.PHONY: help dev-db run dev watch migrate-prod build test clean wrangler-login

help:           ## Show this help message
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

dev-db:         ## Re-export prod D1 into .data/dev.sqlite (requires wrangler login)
	@mkdir -p .data
	@rm -f $(DUMP_FILE) $(DB_FILE)
	wrangler d1 export $(DB_NAME) --remote --output=$(DUMP_FILE)
	sqlite3 $(DB_FILE) < $(DUMP_FILE)
	@echo "Loaded $$(sqlite3 $(DB_FILE) 'SELECT COUNT(*) FROM products') products from $(DB_NAME) into $(DB_FILE)"

run:            ## Start the dev server (uses .env / .env.local if present)
	go run .

dev: dev-db run ## First-time / data-refresh: re-export then start

watch: dev-db   ## Hot reload on .go changes (requires `go install github.com/air-verse/air@latest`)
	@if [ ! -x "$(AIR)" ]; then \
		echo "air not found. Install: go install github.com/air-verse/air@latest"; \
		exit 1; \
	fi
	$(AIR)

migrate-prod:   ## Apply migrations to remote D1
	wrangler d1 execute $(DB_NAME) --remote --file=migrations/0001_init.sql

build:          ## Compile all packages
	go build ./...

test:           ## Run tests
	go test ./...

clean:          ## Remove .data/ and .wrangler/
	rm -rf .data .wrangler

wrangler-login: ## Authenticate wrangler with Cloudflare
	wrangler login
