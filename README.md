# Distill

[![CI](https://github.com/Siddhant-K-code/distill/actions/workflows/ci.yml/badge.svg)](https://github.com/Siddhant-K-code/distill/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Siddhant-K-code/distill)](https://github.com/Siddhant-K-code/distill/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/Siddhant-K-code/distill)](https://goreportcard.com/report/github.com/Siddhant-K-code/distill)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

A reliability layer for LLM context. Deterministic deduplication that removes redundancy before it reaches your model.

```
Data Sources → Distill → LLM
(docs, code, memory, tools)    (reliable outputs)
```

## The Problem

LLM outputs are unreliable because context is polluted.

30-40% of context assembled from multiple sources is semantically redundant. Same information from docs, code, memory, and tools competing for attention. This leads to:

- **Non-deterministic outputs** — Same workflow, different results
- **Confused reasoning** — Signal diluted by repetition
- **Production failures** — Works in demos, breaks at scale

## How It Works

```
Query → Over-fetch (50) → Cluster → Select → MMR Re-rank (8) → LLM
```

1. **Over-fetch** — Retrieve 3-5x more chunks than needed
2. **Cluster** — Group semantically similar chunks (agglomerative clustering)
3. **Select** — Pick best representative from each cluster
4. **MMR Re-rank** — Balance relevance and diversity

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
```

## Configuration

### Environment Variables

```bash
OPENAI_API_KEY      # For text → embedding conversion (see note below)
PINECONE_API_KEY    # For Pinecone backend
QDRANT_URL          # For Qdrant backend (default: localhost:6334)
DISTILL_API_KEYS    # Comma-separated API keys for auth (optional)
```

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
- Bring your own embeddings — include `"embedding"` field in chunks
- Self-host an embedding model — set `EMBEDDING_API_URL` to your endpoint

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

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Your App                           │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                      Distill                            │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐    │
│  │ Fetch   │→ │ Cluster │→ │ Select  │→ │  MMR    │    │
│  │  50     │  │   12    │  │   12    │  │   8     │    │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘    │
│       2ms         6ms         <1ms         3ms          │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                       LLM                               │
└─────────────────────────────────────────────────────────┘
```

## Supported Backends

- **Pinecone** — Fully supported
- **Qdrant** — Fully supported
- **Weaviate** — Coming soon

## Use Cases

- **Code Assistants** — Dedupe context from multiple files/repos
- **RAG Pipelines** — Remove redundant chunks before LLM
- **Agent Workflows** — Clean up tool outputs + memory + docs
- **Enterprise** — Deterministic outputs for compliance

## Why Distill?

| | LLM Compression | Distill |
|---|---|---|
| Latency | ~500ms | ~12ms |
| Deterministic | No | Yes |
| Auditable | No | Yes |
| Lossless | No | Yes |

## Contributing

Contributions welcome! Please read the contributing guidelines first.

```bash
# Run tests
go test ./...

# Build
go build -o distill .
```

## License

AGPL-3.0 — see [LICENSE](LICENSE)

For commercial licensing, contact: siddhantkhare2694@gmail.com

## Links

- [Website](https://distill.siddhantkhare.com)
- [Blog: Engineering Guide to Context Efficiency](https://dev.to/siddhantkcode/the-engineering-guide-to-context-window-efficiency-202b)
- [MCP Configuration](mcp/README.md)
