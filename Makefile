DB_NAME    := krcrackers-products
DUMP_FILE  := .data/prod.sql
DB_FILE    := .data/dev.sqlite
AIR        := $(shell command -v air 2>/dev/null || echo "$$(go env GOPATH 2>/dev/null)/bin/air")

.DEFAULT_GOAL := help

.PHONY: help dev-db run dev stop watch migrate-up migrate-down migrate-status build build-lambda deploy-lambda test clean wrangler-login

help:                ## Show this help message
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

dev-db:              ## Re-export prod D1 into .data/dev.sqlite (requires wrangler login)
	@mkdir -p .data
	@rm -f $(DUMP_FILE) $(DB_FILE)
	wrangler d1 export $(DB_NAME) --remote --output=$(DUMP_FILE)
	sqlite3 $(DB_FILE) < $(DUMP_FILE)
	@echo "Loaded $$(sqlite3 $(DB_FILE) 'SELECT COUNT(*) FROM products') products from $(DB_NAME) into $(DB_FILE)"

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

deploy-lambda: build-lambda  ## Deploy to AWS Lambda
	aws lambda update-function-code \
		--function-name krcrackers \
		--region ap-south-1 \
		--zip-file fileb://lambda.zip

test:                ## Run tests
	go test ./...

clean:               ## Remove .data/ and .wrangler/
	rm -rf .data .wrangler

wrangler-login:      ## Authenticate wrangler with Cloudflare
	wrangler login
