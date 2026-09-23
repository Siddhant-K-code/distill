# Deployment

## Docker

```bash
docker run -p 127.0.0.1:8080:8080 \
  -e OPENAI_API_KEY=sk-... \
  ghcr.io/siddhant-k-code/distill:latest \
  api --memory --session
```

With a persistent volume for memory:

```bash
docker run -p 127.0.0.1:8080:8080 \
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
      - "127.0.0.1:8080:8080"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    command: api --memory --session
    volumes:
      - distill-data:/data

volumes:
  distill-data:
```

Memory and session routes are not covered by API-key authentication. Keep
deployments using `--memory` or `--session` on a trusted network or loopback
listener.

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

A `render.yaml` is included for one-click deployment to the `distill-api`
service. The native Go service must use these commands:

```text
Build Command: go build -o distill-api .
Start Command: ./distill-api api --host 0.0.0.0 --port $PORT
Health Check Path: /health
```

Existing Render services do not automatically adopt Blueprint settings unless
they are managed by that Blueprint. Keep the dashboard start command in sync;
running `./distill-api` without the `api` subcommand prints CLI help and exits
without starting a web server.

The public service leaves persistent memory and session routes disabled. Do not
add `--memory` or `--session` until authentication covers every stateful route.

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
