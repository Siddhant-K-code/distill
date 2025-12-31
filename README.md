# Distill

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

```bash
go install github.com/Siddhant-K-code/distill@latest
```

Or build from source:

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
OPENAI_API_KEY      # Required for text embeddings
PINECONE_API_KEY    # For Pinecone backend
QDRANT_URL          # For Qdrant backend (default: localhost:6334)
DISTILL_API_KEYS    # Comma-separated API keys for auth (optional)
```

### Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `--threshold` | Clustering distance (lower = stricter) | 0.15 |
| `--lambda` | MMR balance: 1.0 = relevance, 0.0 = diversity | 0.5 |
| `--over-fetch-k` | Chunks to retrieve initially | 50 |
| `--target-k` | Chunks to return after dedup | 8 |

## Self-Hosting

### Docker

```bash
docker build -t distill .
docker run -p 8080:8080 -e OPENAI_API_KEY=your-key distill api
```

### Docker Compose

```bash
# Start Distill + Qdrant (local vector DB)
docker-compose up
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

MIT — see [LICENSE](LICENSE)

## Links

- [Blog: Engineering Guide to Context Efficiency](https://dev.to/siddhantkcode/the-engineering-guide-to-context-window-efficiency-202b)
- [MCP Configuration](mcp/README.md)
