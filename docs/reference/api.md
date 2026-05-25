# API Reference

Start the server with `distill api`. Interactive docs available at [/docs](http://localhost:8080/docs).

Full OpenAPI 3.1 spec: [openapi.yaml](../../openapi.yaml)

## Endpoints

### Dedup

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/dedupe` | Deduplicate chunks |
| POST | `/v1/dedupe/stream` | Deduplicate with SSE progress |

### Pipeline

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/pipeline` | Run full dedup → compress → cache pipeline |

### Batch

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/batch` | Submit async batch job |
| GET | `/v1/batch/{job_id}` | Get job status |
| GET | `/v1/batch/{job_id}/results` | Get job results |

### Memory (requires `--memory`)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/memory/store` | Store memories with dedup and sensitivity tagging |
| POST | `/v1/memory/recall` | Recall by relevance + recency |
| POST | `/v1/memory/forget` | Remove by ID, tag, or age |
| POST | `/v1/memory/expire` | Mark as expired (soft delete) |
| POST | `/v1/memory/supersede` | Replace with newer version |
| GET | `/v1/memory/stats` | Store statistics |

### Sessions (requires `--session`)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/session/create` | Create token-budgeted session |
| POST | `/v1/session/push` | Push context entries |
| POST | `/v1/session/context` | Read context window |
| GET | `/v1/session/get` | Get session metadata |
| POST | `/v1/session/delete` | Delete session |

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |
| GET | `/docs` | Swagger UI |
| GET | `/openapi.yaml` | OpenAPI spec |

## Authentication

Set `--api-keys` or `DISTILL_API_KEYS` to enable API key authentication:

```bash
distill api --api-keys "key1,key2"
```

Clients must include the key in the `Authorization` header:

```bash
curl -H "Authorization: Bearer key1" localhost:8080/v1/dedupe -d '{...}'
```
