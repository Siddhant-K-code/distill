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

.PHONY: distill-lock-demo
distill-lock-demo: ## Build, verify, repeat, and mutate the Distill Lock v0 fixture
	$(GO) test ./pkg/lock -run '^TestDistillLockDemo$$' -count=1 -v

.PHONY: distill-jev-pilot
distill-jev-pilot: ## Generate and validate the excluded offline research pilot
	$(GO) run ./cmd/distill-jev-pilot prepare --output build/pilot
	$(GO) run ./cmd/distill-jev-pilot validate build/pilot
	$(GO) run ./cmd/distill-jev-pilot summarize build/pilot
	@printf '%s\n' 'provider_calls=0 final_study_eligible=false'

.PHONY: distill-jev-final
distill-jev-final: ## Generate and validate the offline final-study package
	mkdir -p build
	chmod 700 build
	$(GO) run ./cmd/distill-jev-final prepare \
		--output build/final-study \
		--agenttrace-contamination-artifact research/context-is-a-build-artifact/final-contamination-ledger-v1.md \
		--agenttrace-contamination-sha256 fa2bb7022b30d043d1cabba085253190dba94dd0b85eed98a9cc4fa5e23d0c19
	$(GO) run ./cmd/distill-jev-final validate --input build/final-study
	$(GO) run ./cmd/distill-jev-final summarize --input build/final-study
	@printf '%s\n' 'provider_calls=0 execution_authorized=false'

.PHONY: distill-local-context-control
distill-local-context-control: ## Generate and validate the offline local context-control package
	rm -rf build/local-context-control
	$(GO) run ./cmd/distill-local-context-control prepare --output build/local-context-control
	$(GO) run ./cmd/distill-local-context-control validate --input build/local-context-control
	$(GO) run ./cmd/distill-local-context-control summarize --input build/local-context-control
	@printf '%s\n' 'provider_calls=0 local_observations=0 execution_authorized=false held_out_records=0'

.PHONY: distill-local-context-control-v2
distill-local-context-control-v2: ## Generate and validate the offline local context-control v2 package
	rm -rf build/local-context-control-v2
	$(GO) run ./cmd/distill-local-context-control-v2 prepare --output build/local-context-control-v2
	$(GO) run ./cmd/distill-local-context-control-v2 validate --input build/local-context-control-v2
	$(GO) run ./cmd/distill-local-context-control-v2 summarize --input build/local-context-control-v2
	$(GO) run ./cmd/distill-local-context-control-v2 validate-public --input research/context-is-a-build-artifact
	@printf '%s\n' 'provider_calls=0 local_observations=0 execution_authorized=false held_out_records=0'

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
