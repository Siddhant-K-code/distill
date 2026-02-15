# Distill

[![CI](https://github.com/Siddhant-K-code/distill/actions/workflows/ci.yml/badge.svg)](https://github.com/Siddhant-K-code/distill/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Siddhant-K-code/distill)](https://github.com/Siddhant-K-code/distill/releases/latest)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/Siddhant-K-code/distill)](https://goreportcard.com/report/github.com/Siddhant-K-code/distill)


[![Build with Ona](https://ona.com/build-with-ona.svg)](https://app.ona.com/#https://github.com/siddhant-k-code/distill)

**Context intelligence layer for AI agents.**

Deduplicates, compresses, and manages context across sessions - so your agents produce reliable, deterministic outputs. Today: a dedup pipeline with ~12ms overhead. Next: persistent context memory, code change impact graphs, and session-aware context windows.

Less redundant data. Lower costs. Faster responses. Deterministic results.

**[Learn more →](https://distill.siddhantkhare.com)**

```
Context sources → Distill → LLM
(RAG, tools, memory, docs)    (reliable outputs)
```

## The Problem

LLM outputs are unreliable because context is polluted. "Garbage in, garbage out."

30-40% of context assembled from multiple sources is semantically redundant. Same information from docs, code, memory, and tools competing for attention. This leads to:

- **Non-deterministic outputs** — Same workflow, different results
- **Confused reasoning** — Signal diluted by repetition
- **Production failures** — Works in demos, breaks at scale

You can't fix unreliable outputs with better prompts. You need to fix the context that goes in.

## How It Works

Math, not magic. No LLM calls. Fully deterministic.

| Step | What it does | Benefit |
|------|--------------|---------|
| **Deduplicate** | Remove redundant information across sources | More reliable outputs |
| **Compress** | Keep what matters, remove the noise | Lower token costs |
| **Summarize** | Condense older context intelligently | Longer sessions |
| **Cache** | Instant retrieval for repeated patterns | Faster responses |

### Pipeline

```
Query → Over-fetch (50) → Cluster → Select → MMR Re-rank (8) → LLM
```

1. **Over-fetch** - Retrieve 3-5x more chunks than needed
2. **Cluster** - Group semantically similar chunks (agglomerative clustering)
3. **Select** - Pick best representative from each cluster
4. **MMR Re-rank** - Balance relevance and diversity

**Result:** Deterministic, diverse context in ~12ms. No LLM calls. Fully auditable.

## Installation

### Binary (Recommended)

Download from [GitHub Releases](https://github.com/Siddhant-K-code/distill/releases):

```bash
# macOS (Apple Silicon)
curl -sL $(curl -s https://api.github.com/repos/Siddhant-K-code/distill/releases/latest | grep "browser_download_url.*darwin_arm64.tar.gz" | cut -d '"' -f 4) | tar xz

# macOS (Intel)
curl -sL $(curl -s https://api.github.com/repos/Siddhant-K-code/distill/releases/latest | grep "browser_download_url.*darwin_amd64.tar.gz" | cut -d '"' -f 4) | tar xz

# Linux (amd64)
curl -sL $(curl -s https://api.github.com/repos/Siddhant-K-code/distill/releases/latest | grep "browser_download_url.*linux_amd64.tar.gz" | cut -d '"' -f 4) | tar xz

# Linux (arm64)
curl -sL $(curl -s https://api.github.com/repos/Siddhant-K-code/distill/releases/latest | grep "browser_download_url.*linux_arm64.tar.gz" | cut -d '"' -f 4) | tar xz

# Move to PATH
sudo mv distill /usr/local/bin/
```

Or download directly from the [releases page](https://github.com/Siddhant-K-code/distill/releases/latest).

### Go Install

```bash
go install github.com/Siddhant-K-code/distill@latest
```

### Docker

```bash
docker pull ghcr.io/siddhant-k-code/distill:latest
docker run -p 8080:8080 -e OPENAI_API_KEY=your-key ghcr.io/siddhant-k-code/distill
```

### Build from Source

```bash
git clone https://github.com/Siddhant-K-code/distill.git
cd distill
go build -o distill .
```

## Quick Start

### 1. Standalone API (No Vector DB Required)

Start the API server and send chunks directly:

```bash
export OPENAI_API_KEY="your-key"  # For embeddings
distill api --port 8080
```

Deduplicate chunks:

```bash
curl -X POST http://localhost:8080/v1/dedupe \
  -H "Content-Type: application/json" \
  -d '{
    "chunks": [
      {"id": "1", "text": "React is a JavaScript library for building UIs."},
      {"id": "2", "text": "React.js is a JS library for building user interfaces."},
      {"id": "3", "text": "Vue is a progressive framework for building UIs."}
    ]
  }'
```

Response:

```json
{
  "chunks": [
    {"id": "1", "text": "React is a JavaScript library for building UIs.", "cluster_id": 0},
    {"id": "3", "text": "Vue is a progressive framework for building UIs.", "cluster_id": 1}
  ],
  "stats": {
    "input_count": 3,
    "output_count": 2,
    "reduction_pct": 33,
    "latency_ms": 12
  }
}
```

**With pre-computed embeddings (no OpenAI key needed):**

```bash
curl -X POST http://localhost:8080/v1/dedupe \
  -H "Content-Type: application/json" \
  -d '{
    "chunks": [
      {"id": "1", "text": "React is...", "embedding": [0.1, 0.2, ...]},
      {"id": "2", "text": "React.js is...", "embedding": [0.11, 0.21, ...]},
      {"id": "3", "text": "Vue is...", "embedding": [0.9, 0.8, ...]}
    ]
  }'
```

### 2. With Vector Database

Connect to Pinecone or Qdrant for retrieval + deduplication:

```bash
export PINECONE_API_KEY="your-key"
export OPENAI_API_KEY="your-key"

distill serve --index my-index --port 8080
```

Query with automatic deduplication:

```bash
curl -X POST http://localhost:8080/v1/retrieve \
  -H "Content-Type: application/json" \
  -d '{"query": "how do I reset my password?"}'
```

### 3. MCP Integration (AI Assistants)

Works with Claude, Cursor, Amp, and other MCP-compatible assistants:

```bash
distill mcp
```

Add to Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "distill": {
      "command": "/path/to/distill",
      "args": ["mcp"]
    }
  }
}
```

See [mcp/README.md](mcp/README.md) for more configuration options.

## CLI Commands

```bash
distill api       # Start standalone API server
distill serve     # Start server with vector DB connection
distill mcp       # Start MCP server for AI assistants
distill analyze   # Analyze a file for duplicates
distill sync      # Upload vectors to Pinecone with dedup
distill query     # Test a query from command line
distill config    # Manage configuration files
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/dedupe` | Deduplicate chunks |
| POST | `/v1/dedupe/stream` | SSE streaming dedup with per-stage progress |
| POST | `/v1/retrieve` | Query vector DB with dedup (requires backend) |
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |

## Configuration

### Config File

Distill supports a `distill.yaml` configuration file for persistent settings. Generate a template:

```bash
distill config init              # Creates distill.yaml in current directory
distill config init --stdout     # Print template to stdout
distill config validate          # Validate existing config file
```

Config file search order: `./distill.yaml`, `$HOME/distill.yaml`.

**Priority:** CLI flags > environment variables > config file > defaults.

Example `distill.yaml`:

```yaml
server:
  port: 8080
  host: 0.0.0.0
  read_timeout: 30s
  write_timeout: 60s

embedding:
  provider: openai
  model: text-embedding-3-small
  batch_size: 100

dedup:
  threshold: 0.15
  method: agglomerative
  linkage: average
  lambda: 0.5
  enable_mmr: true

retriever:
  backend: pinecone    # pinecone or qdrant
  index: my-index
  host: ""             # required for qdrant
  namespace: ""
  top_k: 50
  target_k: 8

auth:
  api_keys:
    - ${DISTILL_API_KEY}
```

Environment variables can be referenced using `${VAR}` or `${VAR:-default}` syntax.

### Environment Variables

```bash
OPENAI_API_KEY      # For text → embedding conversion (see note below)
PINECONE_API_KEY    # For Pinecone backend
QDRANT_URL          # For Qdrant backend (default: localhost:6334)
DISTILL_API_KEYS    # Optional: protect your self-hosted instance (see below)
```

### Protecting Your Self-Hosted Instance

If you're exposing Distill publicly, set `DISTILL_API_KEYS` to require authentication:

```bash
# Generate a random API key
export DISTILL_API_KEYS="sk-$(openssl rand -hex 32)"

# Or multiple keys (comma-separated)
export DISTILL_API_KEYS="sk-key1,sk-key2,sk-key3"
```

Then include the key in requests:

```bash
curl -X POST http://your-server:8080/v1/dedupe \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{"chunks": [...]}'
```

If `DISTILL_API_KEYS` is not set, the API is open (suitable for local/internal use).

### About OpenAI API Key

**When you need it:**
- Sending text chunks without pre-computed embeddings
- Using text queries with vector database retrieval
- Using the MCP server with text-based tools

**When you DON'T need it:**
- Sending chunks with pre-computed embeddings (include `"embedding": [...]` in your request)
- Using Distill purely for clustering/deduplication on existing vectors

**What it's used for:**
- Converts text to embeddings using `text-embedding-3-small` model
- ~$0.00002 per 1K tokens (very cheap)
- Embeddings are used only for similarity comparison, never stored

**Alternatives:**
- Bring your own embeddings - include `"embedding"` field in chunks
- Self-host an embedding model - set `EMBEDDING_API_URL` to your endpoint

### Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `--threshold` | Clustering distance (lower = stricter) | 0.15 |
| `--lambda` | MMR balance: 1.0 = relevance, 0.0 = diversity | 0.5 |
| `--over-fetch-k` | Chunks to retrieve initially | 50 |
| `--target-k` | Chunks to return after dedup | 8 |

## Self-Hosting

### Docker (Recommended)

Use the pre-built image from GitHub Container Registry:

```bash
# Pull and run
docker run -p 8080:8080 -e OPENAI_API_KEY=your-key ghcr.io/siddhant-k-code/distill:latest

# Or with a specific version
docker run -p 8080:8080 -e OPENAI_API_KEY=your-key ghcr.io/siddhant-k-code/distill:v0.1.0
```

### Docker Compose

```bash
# Start Distill + Qdrant (local vector DB)
docker-compose up
```

### Build from Source

```bash
docker build -t distill .
docker run -p 8080:8080 -e OPENAI_API_KEY=your-key distill api
```

### Fly.io

```bash
fly launch
fly secrets set OPENAI_API_KEY=your-key
fly deploy
```

### Render

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/Siddhant-K-code/distill)

Or manually:
1. Connect your GitHub repo
2. Set environment variables (`OPENAI_API_KEY`)
3. Deploy

### Railway

Connect your repo and set `OPENAI_API_KEY` in environment variables.

## Monitoring

Distill exposes a Prometheus-compatible `/metrics` endpoint on both `api` and `serve` commands.

### Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `distill_requests_total` | Counter | Total requests by endpoint and status code |
| `distill_request_duration_seconds` | Histogram | Request latency distribution |
| `distill_chunks_processed_total` | Counter | Chunks processed (input/output) |
| `distill_reduction_ratio` | Histogram | Chunk reduction ratio per request |
| `distill_active_requests` | Gauge | Currently processing requests |
| `distill_clusters_formed_total` | Counter | Clusters formed during deduplication |

### Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: distill
    static_configs:
      - targets: ['localhost:8080']
```

### Grafana Dashboard

Import the included dashboard from `grafana/dashboard.json` or use dashboard UID `distill-overview`.

### OpenTelemetry Tracing

Distill supports distributed tracing via OpenTelemetry. Each pipeline stage (embedding, clustering, selection, MMR) is instrumented as a separate span.

Enable via `distill.yaml`:

```yaml
telemetry:
  tracing:
    enabled: true
    exporter: otlp         # otlp, stdout, or none
    endpoint: localhost:4317
    sample_rate: 1.0
    insecure: true
```

Or via environment variables:

```bash
export DISTILL_TELEMETRY_TRACING_ENABLED=true
export DISTILL_TELEMETRY_TRACING_ENDPOINT=localhost:4317
```

Spans emitted per request:

| Span | Attributes |
|------|------------|
| `distill.request` | endpoint |
| `distill.embedding` | chunk_count |
| `distill.clustering` | input_count, threshold |
| `distill.selection` | cluster_count |
| `distill.mmr` | input_count, lambda |
| `distill.retrieval` | top_k, backend |

Result attributes (`distill.result.*`) are added to the root span: input_count, output_count, cluster_count, latency_ms, reduction_ratio.

W3C Trace Context propagation is enabled by default for cross-service tracing.

## Pipeline Modules

### Compression (`pkg/compress`)

Reduces token count while preserving meaning. Three strategies:

- **Extractive** — Scores sentences by position, keyword density, and length; keeps the most salient spans
- **Placeholder** — Replaces verbose JSON, XML, and table outputs with compact structural summaries
- **Pruner** — Strips filler phrases, redundant qualifiers, and boilerplate patterns

Strategies can be chained via `compress.Pipeline`. Configure with target reduction ratio (e.g., 0.3 = keep 30% of original).

### Cache (`pkg/cache`)

KV cache for repeated context patterns (system prompts, tool definitions, boilerplate). Sub-millisecond retrieval for cache hits.

- **MemoryCache** — In-memory LRU with TTL, configurable size limits (entries and bytes), background cleanup
- **PatternDetector** — Identifies cacheable content: system prompts, tool/function definitions, code blocks
- **RedisCache** — Interface for distributed deployments (requires external Redis)

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                         Your App / Agent                             │
└──────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│                             Distill                                  │
│                                                                      │
│  Dedup Pipeline (shipped)                                            │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌──────────┐  ┌─────────┐  │
│  │  Cache  │→ │ Cluster │→ │ Select  │→ │ Compress │→ │  MMR    │  │
│  │  check  │  │  dedup  │  │  best   │  │  prune   │  │ re-rank │  │
│  └─────────┘  └─────────┘  └─────────┘  └──────────┘  └─────────┘  │
│     <1ms          6ms         <1ms          2ms           3ms        │
│                                                                      │
│  Context Intelligence (planned)                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐   │
│  │ Memory Store │  │ Impact Graph │  │ Session Context Windows  │   │
│  │  (#29)       │  │  (#30)       │  │  (#31)                   │   │
│  └──────────────┘  └──────────────┘  └──────────────────────────┘   │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │  /metrics (Prometheus)  ·  OTEL tracing  ·  MCP server      │    │
│  └──────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│                              LLM                                     │
└──────────────────────────────────────────────────────────────────────┘
```

## Supported Backends

- **Pinecone** - Fully supported
- **Qdrant** - Fully supported
- **Weaviate** - Coming soon

## Use Cases

- **Code Assistants** - Dedupe context from multiple files/repos
- **RAG Pipelines** - Remove redundant chunks before LLM
- **Agent Workflows** - Clean up tool outputs + memory + docs
- **Incident Triage** - Find similar past changes that caused outages
- **Code Review** - Blast radius analysis for PRs
- **Enterprise** - Deterministic outputs with source attribution

## Roadmap

Distill is evolving from a dedup utility into a context intelligence layer. Here's what's next:

### Context Memory

| Feature | Issue | Description |
|---------|-------|-------------|
| **Context Memory Store** | [#29](https://github.com/Siddhant-K-code/distill/issues/29) | Persistent, deduplicated memory across sessions. Write-time dedup, hierarchical decay (full text -> summary -> keywords -> evicted), token-budgeted recall. |
| **Session Management** | [#31](https://github.com/Siddhant-K-code/distill/issues/31) | Stateful context windows for long-running agents. Push context incrementally, Distill keeps it deduplicated and within budget. |

### Code Intelligence

| Feature | Issue | Description |
|---------|-------|-------------|
| **Change Impact Graph** | [#30](https://github.com/Siddhant-K-code/distill/issues/30) | Dependency graph + co-change patterns from git history. "This PR changes auth/jwt.go - here's the blast radius." |
| **Semantic Commit Analysis** | [#32](https://github.com/Siddhant-K-code/distill/issues/32) | Find similar past changes, predict incidents. "This diff is 82% similar to the one that caused outage #47." |

### Infrastructure

| Feature | Issue | Description |
|---------|-------|-------------|
| **Multi-Provider Embeddings** | [#33](https://github.com/Siddhant-K-code/distill/issues/33) | Ollama, Azure OpenAI, Cohere, HuggingFace. Swap providers via config. |
| **Batch API** | [#11](https://github.com/Siddhant-K-code/distill/issues/11) | Async batch processing for large workloads. |
| **Python SDK** | [#5](https://github.com/Siddhant-K-code/distill/issues/5) | `pip install distill-ai` with LangChain/LlamaIndex integrations. |
| **OpenAPI Spec** | [#23](https://github.com/Siddhant-K-code/distill/issues/23) | Swagger UI at `/docs`, auto-generated client SDKs. |

See all open issues: [github.com/Siddhant-K-code/distill/issues](https://github.com/Siddhant-K-code/distill/issues)

## Why not just use an LLM?

LLMs are non-deterministic. Reliability requires deterministic preprocessing.

| | LLM Compression | Distill |
|---|---|---|
| Latency | ~500ms | ~12ms |
| Cost per call | $0.01+ | $0.0001 |
| Deterministic | No | Yes |
| Lossless | No | Yes |
| Auditable | No | Yes |

Use LLMs for reasoning. Use deterministic algorithms for reliability.

## Integrations

Works with your existing AI stack:

- **LLM Providers:** OpenAI, Anthropic (more via [#33](https://github.com/Siddhant-K-code/distill/issues/33))
- **Frameworks:** LangChain, LlamaIndex (SDKs planned: [#5](https://github.com/Siddhant-K-code/distill/issues/5))
- **Vector DBs:** Pinecone, Qdrant
- **AI Assistants:** Claude Desktop, Cursor (via MCP)
- **Observability:** Prometheus, Grafana, OpenTelemetry (Jaeger, Tempo)

## Contributing

Contributions welcome! Check the [open issues](https://github.com/Siddhant-K-code/distill/issues) for things to work on.

```bash
git clone https://github.com/Siddhant-K-code/distill.git
cd distill
go build -o distill .
go test ./...
```

## License

AGPL-3.0 - see [LICENSE](LICENSE)

For commercial licensing, contact: siddhantkhare2694@gmail.com

## Links

- [Website](https://distill.siddhantkhare.com)
- [Playground](https://distill.siddhantkhare.com/playground)
- [Blog Post](https://dev.to/siddhantkcode/the-engineering-guide-to-context-window-efficiency-202b)
- [MCP Configuration](mcp/README.md)
- [Book a Demo](https://meet.siddhantkhare.com)

