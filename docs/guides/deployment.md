# Deployment

## Docker

```bash
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=sk-... \
  ghcr.io/siddhant-k-code/distill:latest \
  api --memory --session
```

With a persistent volume for memory:

```bash
docker run -p 8080:8080 \
  -v distill-data:/data \
  -e OPENAI_API_KEY=sk-... \
  ghcr.io/siddhant-k-code/distill:latest \
  api --memory --memory-db /data/memory.db --session --session-db /data/sessions.db
```

## Docker Compose

```yaml
version: "3.8"
services:
  distill:
    image: ghcr.io/siddhant-k-code/distill:latest
    ports:
      - "8080:8080"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    command: api --memory --session
    volumes:
      - distill-data:/data

volumes:
  distill-data:
```

## Binary

Download from [GitHub Releases](https://github.com/Siddhant-K-code/distill/releases) and run directly:

```bash
distill api --memory --session
```

## Fly.io

A `fly.toml` is included in the repository:

```bash
fly launch
fly secrets set OPENAI_API_KEY=sk-...
fly deploy
```

## Render

A `render.yaml` is included for one-click deployment to Render.

## Environment variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | OpenAI API key for embeddings |
| `COHERE_API_KEY` | Cohere API key (when using `--embedding-provider cohere`) |
| `DISTILL_API_KEYS` | Comma-separated API keys for authentication |

## Observability

### Prometheus metrics

Available at `/metrics`:

```bash
curl localhost:8080/metrics
```

### OpenTelemetry tracing

```bash
distill api --otel-endpoint localhost:4317
# or
distill api --otel-stdout  # print traces to stdout
```

### Grafana

Import the dashboard template from `grafana/dashboard.json`.
