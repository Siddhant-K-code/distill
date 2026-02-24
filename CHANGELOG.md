# Changelog

All notable changes to Distill are documented here.

## [Unreleased]

### Added

- **Session-based context window management** (`pkg/session`) — Token-budgeted context windows for long-running agent sessions. Entries are deduplicated on push, compressed through hierarchical levels (full text → summary → sentence → keywords), and evicted when the budget is exceeded. Lowest-importance entries are compressed first. ([#38](https://github.com/Siddhant-K-code/distill/pull/38), closes [#31](https://github.com/Siddhant-K-code/distill/issues/31))
- **Session CLI** — `distill session create/push/context/delete` commands. ([#38](https://github.com/Siddhant-K-code/distill/pull/38))
- **Session HTTP API** — `/v1/session/create`, `/push`, `/context`, `/delete`, `/get` endpoints. Opt-in via `--session` flag. ([#38](https://github.com/Siddhant-K-code/distill/pull/38))
- **Session MCP tools** — `create_session`, `push_session`, `session_context`, `delete_session` for Claude Desktop, Cursor, and Amp. Opt-in via `--session` flag. ([#38](https://github.com/Siddhant-K-code/distill/pull/38))

### Stats

- 9 files changed, 1,928 insertions, 6 deletions
- 1 new package: `pkg/session`
- 13 new tests

---

## [v0.3.0] - 2026-02-23

Feature release adding persistent context memory, SSE streaming, OpenTelemetry tracing, and project documentation.

### Added

- **Persistent context memory store** (`pkg/memory`) — SQLite-backed memory that persists across agent sessions. Write-time deduplication via cosine similarity, recall ranked by `(1-w)*similarity + w*recency`, tag filtering via junction table, and token-budgeted results. Opt-in via `--memory` flag on `api` and `mcp` commands. ([#37](https://github.com/Siddhant-K-code/distill/pull/37), closes [#29](https://github.com/Siddhant-K-code/distill/issues/29))
- **Hierarchical decay worker** — Background compression of aging memories: full text → summary (extractive, ~20%) → keywords (~5%) → evicted. Configurable ages via `distill.yaml`. Accessing a memory resets its decay clock. ([#37](https://github.com/Siddhant-K-code/distill/pull/37))
- **Memory CLI** — `distill memory store/recall/forget/stats` commands for direct memory management. ([#37](https://github.com/Siddhant-K-code/distill/pull/37))
- **Memory HTTP API** — `POST /v1/memory/store`, `/recall`, `/forget`, `GET /stats` endpoints. ([#37](https://github.com/Siddhant-K-code/distill/pull/37))
- **Memory MCP tools** — `store_memory`, `recall_memory`, `forget_memory`, `memory_stats` tools for Claude Desktop, Cursor, and Amp. ([#37](https://github.com/Siddhant-K-code/distill/pull/37))
- **SSE streaming dedup** (`pkg/sse`) — `POST /v1/dedupe/stream` endpoint with per-stage progress events (embedding, clustering, selection, MMR). ([#22](https://github.com/Siddhant-K-code/distill/pull/22))
- **OpenTelemetry tracing** (`pkg/telemetry`) — Distributed tracing with OTLP and stdout exporters. Each pipeline stage instrumented as a separate span. W3C Trace Context propagation. ([#21](https://github.com/Siddhant-K-code/distill/pull/21))
- **FAQ** (`FAQ.md`) — 20 Q&As covering algorithms, integrations, deployment, and cost. ([#36](https://github.com/Siddhant-K-code/distill/pull/36))

### Changed

- **README** — Added Context Memory section with CLI/API/MCP examples, updated architecture diagram (Memory Store: shipped), updated roadmap status, expanded API endpoints table. ([#37](https://github.com/Siddhant-K-code/distill/pull/37), [#34](https://github.com/Siddhant-K-code/distill/pull/34))

### Dependencies

- Added `modernc.org/sqlite` — Pure Go SQLite driver (no CGO required)

### Stats

- 21 files changed, 3,617 insertions, 73 deletions
- 3 new packages: `pkg/memory`, `pkg/sse`, `pkg/telemetry`
- 11 new tests for memory store

---

## [v0.2.0] - 2026-02-14

Major release adding four new modules: semantic compression, KV caching, Prometheus observability, and YAML configuration.

### Added

- **Prometheus metrics endpoint** (`pkg/metrics`) — `/metrics` endpoint on both `api` and `serve` commands. Tracks request rate, latency percentiles, chunks processed, reduction ratio, active requests, and clusters formed. Includes HTTP middleware for automatic instrumentation. ([#19](https://github.com/Siddhant-K-code/distill/pull/19))
- **Grafana dashboard** (`grafana/dashboard.json`) — 10-panel dashboard template covering request rate, error rate, P99 latency, latency percentiles, chunks processed, reduction ratio, clusters formed, and status code breakdown. ([#19](https://github.com/Siddhant-K-code/distill/pull/19))
- **Configuration file support** (`pkg/config`) — `distill.yaml` config file with schema validation and `${VAR:-default}` environment variable interpolation. New CLI commands: `distill config init` and `distill config validate`. Config priority: CLI flags > env vars > config file > defaults. ([#18](https://github.com/Siddhant-K-code/distill/pull/18))
- **Semantic compression module** (`pkg/compress`) — Three compression strategies: extractive (sentence scoring), placeholder (JSON/XML/table summarization), and pruner (filler phrase removal). Chainable via `compress.Pipeline`. ([#13](https://github.com/Siddhant-K-code/distill/pull/13))
- **KV cache for repeated patterns** (`pkg/cache`) — In-memory LRU cache with TTL support for system prompts, tool definitions, and other repeated context. Includes `PatternDetector` for automatic identification of cacheable content and a Redis interface for distributed deployments. ([#17](https://github.com/Siddhant-K-code/distill/pull/17))
- **GitHub Sponsors funding configuration** ([5b11d6d](https://github.com/Siddhant-K-code/distill/commit/5b11d6d))

### Changed

- **Architecture diagram** updated to reflect the full pipeline: Cache check → Cluster dedup → Select → Compress → MMR re-rank
- **README** expanded with pipeline module docs (compression, cache), monitoring section, config file usage, and updated integrations table ([#15](https://github.com/Siddhant-K-code/distill/pull/15))
- **Viper config loading** now searches for `distill.yaml` (previously `.distill`), uses `DISTILL_` env prefix with key replacer ([#18](https://github.com/Siddhant-K-code/distill/pull/18))
- **`serve.go`** — `/metrics` endpoint now serves Prometheus format instead of JSON config dump ([#19](https://github.com/Siddhant-K-code/distill/pull/19))

### Fixed

- **Docker workflow permissions** — Added attestations and id-token write permissions for ghcr.io push ([#14](https://github.com/Siddhant-K-code/distill/pull/14))

### Stats

- 24 files changed, 3,572 insertions, 55 deletions
- 4 new packages: `pkg/compress`, `pkg/cache`, `pkg/config`, `pkg/metrics`
- 46 new tests across all packages

---

## [v0.1.2] - 2026-01-01

### Fixed

- Go Report Card badge and formatting
- gofmt simplifications applied
- Documentation em-dash replacements

## [v0.1.1] - 2025-12-28

### Added

- Hosted API link in README

## [v0.1.0] - 2025-12-27

Initial release.

- Core deduplication pipeline: agglomerative clustering + MMR re-ranking
- Standalone API server (`distill api`)
- Vector DB server (`distill serve`) with Pinecone and Qdrant backends
- MCP server for AI assistants (`distill mcp`)
- Bulk sync and analyze commands
- Docker image, Fly.io and Render deployment configs
- GoReleaser cross-platform builds
