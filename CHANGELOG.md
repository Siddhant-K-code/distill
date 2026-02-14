# Changelog

All notable changes to Distill are documented here.

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
