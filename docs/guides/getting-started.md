# Getting Started

## Install

### Binary (recommended)

Download from [GitHub Releases](https://github.com/Siddhant-K-code/distill/releases):

```bash
# macOS / Linux
curl -sSL https://github.com/Siddhant-K-code/distill/releases/latest/download/distill_$(uname -s)_$(uname -m).tar.gz | tar xz
sudo mv distill /usr/local/bin/
```

### Docker

```bash
docker pull ghcr.io/siddhant-k-code/distill:latest
```

### Build from source

```bash
git clone https://github.com/Siddhant-K-code/distill.git
cd distill
make build
```

## Quick start

### 1. Start the API server

```bash
# With OpenAI embeddings
export OPENAI_API_KEY=sk-...
distill api

# With Ollama (no API key needed)
distill api --embedding-provider ollama --embedding-model nomic-embed-text

# With memory and sessions enabled
distill api --memory --session
```

### 2. Deduplicate context

```bash
curl -X POST http://localhost:8080/v1/dedupe -H "Content-Type: application/json" -d '{
  "chunks": [
    {"id": "1", "text": "JWT tokens use RS256 signing for authentication"},
    {"id": "2", "text": "Authentication is handled via JWT with RS256 signatures"},
    {"id": "3", "text": "The database uses PostgreSQL 15 with pgvector"}
  ],
  "threshold": 0.15
}'
```

Chunks 1 and 2 are semantically identical — Distill keeps one and returns it alongside chunk 3.

### 3. Run the full pipeline

```bash
curl -X POST http://localhost:8080/v1/pipeline -H "Content-Type: application/json" -d '{
  "chunks": [
    {"id": "1", "text": "..."},
    {"id": "2", "text": "..."}
  ],
  "options": {
    "dedup": true,
    "compress": true,
    "cache": true
  }
}'
```

### 4. Store a memory

```bash
curl -X POST http://localhost:8080/v1/memory/store -H "Content-Type: application/json" -d '{
  "entries": [{
    "text": "The auth service uses JWT with RS256",
    "source": "code_review",
    "tags": ["auth", "architecture"],
    "auto_classify": true
  }]
}'
```

### 5. Recall memories

```bash
curl -X POST http://localhost:8080/v1/memory/recall -H "Content-Type: application/json" -d '{
  "query": "how does authentication work?",
  "max_results": 5,
  "boost_tags": ["auth"]
}'
```

## Configuration

Create `~/.distill.yaml` or `distill.yaml` in your project:

```yaml
server:
  port: 8080
  host: 0.0.0.0

embedding:
  provider: openai       # openai, ollama, or cohere
  model: text-embedding-3-small
  # base_url: http://localhost:11434  # for Ollama

memory:
  db_path: distill-memory.db
  dedup_threshold: 0.15
  conflict_threshold: 0.35

pipeline:
  dedup: true
  compress: true
  cache: true
```

Run `distill config init` to generate a default config file.

## Interactive docs

Start the server and open [http://localhost:8080/docs](http://localhost:8080/docs) for the Swagger UI API explorer.

## Next steps

- [Memory guide](memory.md) — persistent context across sessions
- [Sessions guide](sessions.md) — token-budgeted context windows
- [MCP integration](mcp.md) — use with Claude Desktop and Cursor
