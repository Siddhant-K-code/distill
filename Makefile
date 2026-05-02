BINARY     := distill
GO         := go
GOFLAGS    :=
LDFLAGS    := -s -w
BUILD_DIR  := .

.DEFAULT_GOAL := build

# ── Build ─────────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build the distill binary
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) .

.PHONY: install
install: ## Install distill to $GOPATH/bin
	$(GO) install $(GOFLAGS) -ldflags "$(LDFLAGS)" .

.PHONY: clean
clean: ## Remove build artifacts
	rm -f $(BUILD_DIR)/$(BINARY)

# ── Test ──────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all tests
	$(GO) test ./...

.PHONY: test-verbose
test-verbose: ## Run all tests with verbose output
	$(GO) test -v ./...

.PHONY: test-race
test-race: ## Run tests with race detector
	$(GO) test -race ./...

.PHONY: test-cover
test-cover: ## Run tests and show coverage
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
	rm -f coverage.out

.PHONY: bench
bench: ## Run benchmarks
	$(GO) test -bench=. -benchmem ./...

# ── Code quality ──────────────────────────────────────────────────────────────

.PHONY: fmt
fmt: ## Format all Go source files
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## Run golangci-lint (requires golangci-lint in PATH)
	golangci-lint run ./...

.PHONY: check
check: fmt vet test ## Run fmt, vet, and test

# ── Docker ────────────────────────────────────────────────────────────────────

.PHONY: docker-build
docker-build: ## Build the Docker image
	docker build -t $(BINARY):latest .

.PHONY: docker-run
docker-run: ## Run the Docker container (API mode, port 8080)
	docker run --rm -p 8080:8080 \
		-e OPENAI_API_KEY="$$OPENAI_API_KEY" \
		$(BINARY):latest api

# ── Release ───────────────────────────────────────────────────────────────────

.PHONY: release-dry
release-dry: ## Dry-run goreleaser (snapshot, no publish)
	goreleaser release --snapshot --clean

.PHONY: release
release: ## Run goreleaser (requires GITHUB_TOKEN)
	goreleaser release --clean

# ── Dev helpers ───────────────────────────────────────────────────────────────

.PHONY: config-init
config-init: build ## Generate a default distill.yaml
	./$(BINARY) config init

.PHONY: run-api
run-api: build ## Start the API server on :8080
	./$(BINARY) api

.PHONY: run-serve
run-serve: build ## Start the serve command
	./$(BINARY) serve

.PHONY: deps
deps: ## Download and tidy Go modules
	$(GO) mod download
	$(GO) mod tidy

# ── Help ──────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## List all available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' \
		| sort
