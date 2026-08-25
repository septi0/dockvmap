BINARY := bin/dockvmap
CONFIG := config/config.yaml
FILTERS := config/filters.yaml
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)

.DEFAULT_GOAL := help
.PHONY: build build-backend build-frontend frontend-deps run dev dev-backend dev-frontend test vet fmt lint check clean help

build: build-frontend build-backend ## Build frontend + backend (bin/dockvmap)

build-backend: frontend/dist ## Build the Go binary only
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/dockvmap

build-frontend: ## npm install + build frontend/dist
	cd frontend && npm install && npm run build

frontend/dist: frontend/node_modules
	cd frontend && npm run build

frontend/node_modules: frontend/package.json frontend/package-lock.json
	cd frontend && npm install

frontend-deps: ## npm install in frontend/ only
	cd frontend && npm install

run: build-backend ## Build then run the backend binary
	./$(BINARY) -config $(CONFIG) -filters $(FILTERS)

dev: ## Run backend + frontend dev server together (Ctrl+C stops both)
	$(MAKE) -j2 dev-backend dev-frontend

dev-backend: frontend/dist
	go run -ldflags "-X main.version=$(VERSION)" ./cmd/dockvmap -config $(CONFIG) -filters $(FILTERS)

dev-frontend: frontend/node_modules
	cd frontend && npm run dev

test: ## go test ./...
	go test ./...

vet: ## go vet ./...
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

check: ## Frontend svelte-check + tsc
	cd frontend && npm run check

clean: ## Remove bin/dockvmap and frontend/dist
	rm -rf $(BINARY) frontend/dist

help: ## Show this help
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
