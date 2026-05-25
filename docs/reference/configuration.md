# Configuration

Distill reads configuration from CLI flags, environment variables, and a config file (`~/.distill.yaml`).

## Precedence

CLI flags > environment variables > config file > defaults.

## Config file

```yaml
# ~/.distill.yaml
embedding:
  provider: openai        # openai | ollama | cohere
  model: text-embedding-3-small
  base_url: ""            # override for ollama/custom endpoints

memory:
  db_path: ~/.distill/memory.db
  dedup_threshold: 0.15   # cosine distance below which chunks are duplicates
  conflict_threshold: 0.35
  decay_rate: 0.01

session:
  db_path: ~/.distill/sessions.db

server:
  port: 8080
  api_keys: []
```

## CLI flags

### `distill api`

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--port` | `PORT` | `8080` | Server port |
| `--api-keys` | `DISTILL_API_KEYS` | — | Comma-separated API keys |
| `--memory` | — | `false` | Enable memory subsystem |
| `--memory-db` | — | `~/.distill/memory.db` | SQLite path for memory |
| `--session` | — | `false` | Enable session subsystem |
| `--session-db` | — | `~/.distill/sessions.db` | SQLite path for sessions |
| `--embedding-provider` | — | `openai` | Embedding provider |
| `--embedding-model` | — | `text-embedding-3-small` | Embedding model |
| `--embedding-base-url` | — | — | Custom base URL |
| `--otel-endpoint` | — | — | OTLP gRPC endpoint |
| `--otel-stdout` | — | `false` | Print traces to stdout |

### `distill serve` (MCP mode)

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--memory` | — | `false` | Enable memory tools |
| `--memory-db` | — | `~/.distill/memory.db` | SQLite path |
| `--embedding-provider` | — | `openai` | Embedding provider |
| `--embedding-model` | — | `text-embedding-3-small` | Embedding model |
| `--embedding-base-url` | — | — | Custom base URL |

### `distill memory`

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | `~/.distill/memory.db` | SQLite path |
| `--embedding-provider` | `openai` | Embedding provider |
| `--embedding-model` | `text-embedding-3-small` | Embedding model |
| `--embedding-base-url` | — | Custom base URL |

## Environment variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | OpenAI API key |
| `COHERE_API_KEY` | Cohere API key |
| `DISTILL_API_KEYS` | Comma-separated API keys for auth |
| `PORT` | Server port |
