BINARY            := bin/dockvmap
VERSION           ?= $(shell git describe --tags --always 2>/dev/null || echo 0.0.0-dev)
DEFAULT_DATA_PATH ?= ./data
CONFIG            ?= config/config.yaml
LDFLAGS           := -X main.version=$(VERSION) -X main.defaultDataPath=$(DEFAULT_DATA_PATH)

.DEFAULT_GOAL := help
.PHONY: build build-frontend frontend-deps dev dev-backend dev-frontend \
        test vet fmt lint check verify clean help

build: build-frontend ## Build the frontend and the Go binary (bin/dockvmap)
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/dockvmap

build-frontend: frontend/node_modules ## Build frontend/dist
	cd frontend && npm run build

frontend-deps: frontend/node_modules ## Install frontend dependencies

# reinstalled only when the dependencies themselves change
frontend/node_modules: frontend/package.json frontend/package-lock.json
	cd frontend && npm ci
	@touch $@

dev: ## Run backend + frontend dev server together (Ctrl+C stops both); CONFIG=<path> for another config
	$(MAKE) -j2 dev-backend dev-frontend CONFIG=$(CONFIG)

dev-backend: build-frontend $(CONFIG)
	go run -ldflags "$(LDFLAGS)" ./cmd/dockvmap -config $(CONFIG)

dev-frontend: frontend/node_modules
	cd frontend && npm run dev

config/config.yaml:
	mkdir -p config
	cp config.sample.yaml $@

verify: lint test check ## lint + Go tests + frontend check

# frontend/embed.go embeds frontend/dist, so Go can't compile without it
test: build-frontend ## go test -race ./...
	go test -race ./...

vet: build-frontend ## go vet ./...
	go vet ./...

fmt: ## gofmt -w .
	gofmt -w .

lint: vet ## gofmt check + go vet (+ golangci-lint if installed)
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then echo "gofmt needed on:"; echo "$$files"; exit 1; fi
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping (gofmt + go vet passed)"; \
	fi

check: frontend/node_modules ## Frontend svelte-check + tsc
	cd frontend && npm run check

clean: ## Remove bin/ and frontend/dist
	rm -rf $(BINARY) frontend/dist

help: ## Show this help
	@echo "Usage: make <target> [CONFIG=path] [VERSION=x] [DEFAULT_DATA_PATH=path]"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
