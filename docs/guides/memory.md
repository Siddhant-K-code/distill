# Memory

Distill provides persistent context memory that survives across sessions. Memories are deduplicated at write time, ranked by relevance and recency at recall, and automatically classified for sensitivity.

## Enable memory

```bash
distill api --memory
# or
distill mcp --memory
```

## Store

```bash
curl -X POST localhost:8080/v1/memory/store -d '{
  "entries": [{
    "text": "Auth uses JWT with RS256 signing",
    "source": "code_review",
    "tags": ["auth", "security"],
    "auto_classify": true
  }]
}'
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `text` | string | Memory content (required) |
| `embedding` | float[] | Pre-computed embedding (optional — server can generate) |
| `source` | string | Origin of the memory (e.g. `code_review`, `docs`) |
| `tags` | string[] | Categorization tags |
| `metadata` | object | Arbitrary key-value metadata |
| `sensitivity` | int | Explicit sensitivity level (0-3) |
| `auto_classify` | bool | Run pattern-based sensitivity classification |
| `expires_at` | datetime | TTL — memory excluded from recall after this time |

### Sensitivity levels

| Level | Value | Description |
|-------|-------|-------------|
| None | 0 | No sensitive content |
| PII | 1 | Email, phone, credit card, SSN |
| Internal | 2 | Internal domains, pricing, roadmaps |
| Credentials | 3 | API keys, tokens, passwords |

When `auto_classify: true`, Distill scans the text for patterns (AWS keys, OpenAI keys, GitHub tokens, emails, etc.) and sets the sensitivity level automatically. Explicit `sensitivity` takes precedence if higher.

### Deduplication

If a new entry's embedding is within the dedup threshold (default: 0.15 cosine distance) of an existing entry, it's merged — the existing entry's `last_referenced` and `access_count` are updated.

### Conflict detection

If a new entry is similar but not identical (between dedup threshold and conflict threshold), it's stored AND flagged:

```json
{
  "stored": 1,
  "conflicts": [{
    "new_id": "abc123",
    "new_text": "Auth uses HMAC with HS256",
    "existing_id": "def456",
    "existing_text": "Auth uses JWT with RS256",
    "distance": 0.23
  }]
}
```

The caller can resolve by superseding the old entry.

## Recall

```bash
curl -X POST localhost:8080/v1/memory/recall -d '{
  "query": "how does authentication work?",
  "max_results": 5,
  "boost_tags": ["auth"],
  "min_relevance": 0.3,
  "task_context": "fixing login bugs in code_review"
}'
```

### Ranking

Relevance is computed as:

```
score = (1 - recency_weight) × similarity + recency_weight × recency
      + 0.1  if any tag matches boost_tags
      + 0.05 if source appears in task_context
```

### Response

```json
{
  "memories": [...],
  "max_sensitivity": 1,
  "sensitive_chunks": [
    {"chunk_id": "abc123", "sensitivity": 1}
  ],
  "stats": {
    "candidates": 42,
    "returned": 5,
    "token_count": 1200
  }
}
```

## Expire

Mark memories as expired without deleting them:

```bash
curl -X POST localhost:8080/v1/memory/expire -d '{
  "ids": ["abc123", "def456"]
}'
```

Expired entries are excluded from recall by default. Use `"include_expired": true` in recall to retrieve them.

## Supersede

Replace an outdated memory with a newer one:

```bash
curl -X POST localhost:8080/v1/memory/supersede -d '{
  "old_id": "abc123",
  "new_id": "ghi789"
}'
```

The old entry is expired and a forward pointer to the replacement is stored.

## Forget

Permanently remove memories:

```bash
# By ID
curl -X POST localhost:8080/v1/memory/forget -d '{"ids": ["abc123"]}'

# By tag
curl -X POST localhost:8080/v1/memory/forget -d '{"tags": ["temp"]}'

# By age
curl -X POST localhost:8080/v1/memory/forget -d '{"before": "2026-01-01T00:00:00Z"}'
```

## Decay

Memories decay over time through four levels:

1. **Full** — complete text preserved
2. **Summary** — extractive summary
3. **Keywords** — key terms only
4. **Evicted** — removed from store

Decay is automatic when enabled (`decay_enabled: true` in config). Frequently accessed memories resist decay.
