# Sessions

Sessions provide token-budgeted context windows for long-running agents. Push context incrementally — Distill deduplicates and compresses to stay within the budget.

## Enable sessions

```bash
distill api --session
# or with a custom database path
distill api --session --session-db my-sessions.db
```

## Create a session

```bash
curl -X POST localhost:8080/v1/session/create -d '{
  "max_tokens": 4000,
  "dedup_threshold": 0.15,
  "preserve_recent": 3
}'
```

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Auto-generated if empty |
| `max_tokens` | int | Token budget for the context window |
| `dedup_threshold` | float | Cosine distance threshold for dedup (default: 0.15) |
| `preserve_recent` | int | Always keep last N entries at full fidelity |

## Push context

```bash
curl -X POST localhost:8080/v1/session/push -d '{
  "session_id": "sess_abc123",
  "entries": [
    {"role": "user", "content": "Fix the login bug in auth.go"},
    {"role": "assistant", "content": "I found the issue in the JWT validation..."}
  ]
}'
```

Response:

```json
{
  "added": 2,
  "deduplicated": 0,
  "compressed": 1,
  "tokens_used": 3200,
  "tokens_remaining": 800
}
```

When the token budget is exceeded, older entries are compressed (summary → keywords) to make room.

## Read context

```bash
curl -X POST localhost:8080/v1/session/context -d '{
  "session_id": "sess_abc123",
  "max_tokens": 2000,
  "role": "assistant"
}'
```

Returns entries with their compression level:

```json
{
  "entries": [
    {"role": "user", "content": "Fix the login bug...", "compression_level": "full", "tokens": 120},
    {"role": "assistant", "content": "JWT validation issue...", "compression_level": "summary", "tokens": 45}
  ],
  "total_tokens": 165
}
```

## Get session metadata

```bash
curl "localhost:8080/v1/session/get?session_id=sess_abc123"
```

## Delete a session

```bash
curl -X POST localhost:8080/v1/session/delete -d '{
  "session_id": "sess_abc123"
}'
```

## How compression works

As the context window fills up, Distill compresses older entries:

1. **Recent entries** (last N, configurable) — kept at full fidelity
2. **Medium-age entries** — extractive summary
3. **Old entries** — keywords only
4. **Over budget** — evicted

This ensures the most recent context is always complete while older context is preserved in compressed form.
