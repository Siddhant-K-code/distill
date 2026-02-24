# Frequently Asked Questions

## General

### What does Distill do?

Distill is a post-retrieval processing layer for RAG pipelines. When you fetch chunks from a vector database, 30-40% are typically redundant - same information phrased differently. Distill clusters semantically similar chunks, picks the best representative from each cluster, compresses verbose content, and re-ranks for diversity. Total overhead is ~12ms. No LLM calls.

### Why not just fetch fewer results from the vector DB?

Fetching fewer results risks missing relevant information. The better approach is to over-fetch (retrieve 20-50 results) and then intelligently deduplicate. This casts a wide net for recall, then optimizes for precision and diversity.

### Is this just removing exact duplicates?

No. Exact dedup is trivial (hash comparison). Distill does _semantic_ dedup - it identifies chunks that convey the same information in different words. Two paragraphs explaining "how JWT auth works" with different wording will be clustered together, and only the best one is kept.

### Why not use an LLM for compression?

LLMs are non-deterministic. The same input can produce different compressed outputs across runs. Distill uses deterministic algorithms (cosine distance, agglomerative clustering, MMR) so the same input always produces the same output. It's also ~40x faster (~12ms vs ~500ms) and ~100x cheaper per call.

---

### What is Context Memory?

Persistent memory that accumulates knowledge across agent sessions. Store context once, recall it later by semantic similarity + recency. Memories are deduplicated on write and compressed over time through hierarchical decay (full text → summary → keywords → evicted). Enable with `--memory` on the `api` or `mcp` commands.

### What are Sessions?

Token-budgeted context windows for long-running agent tasks. Push context incrementally as the agent works - Distill deduplicates entries, compresses aging ones, and evicts when the budget is exceeded. The `preserve_recent` setting keeps the N most recent entries at full fidelity. Enable with `--session` on the `api` or `mcp` commands.

### How is Context Memory different from Sessions?

Memory is cross-session: knowledge persists after a session ends and can be recalled in future sessions. Sessions are within-task: a bounded context window that tracks what the agent has seen during a single task, enforcing a token budget. Use memory for long-term knowledge, sessions for working context.

## Algorithms

### Why agglomerative clustering instead of K-Means?

K-Means requires specifying K upfront and assumes spherical clusters. Agglomerative clustering adapts to the data - it stops merging when the distance between the closest clusters exceeds the threshold. If your 20 chunks have 8 natural groups, you get 8 clusters. If they have 15, you get 15. No tuning required.

### What does the threshold of 0.15 mean?

Cosine distance of 0.15 means cosine similarity of 0.85. Two chunks with 85%+ similarity are considered "saying the same thing." For code, use 0.10 (stricter - code is more precise). For prose, use 0.20 (looser - natural language has more variation).

### How does MMR (Maximal Marginal Relevance) work?

MMR greedily selects chunks that balance relevance and diversity:

```
MMR(chunk) = λ × relevance - (1-λ) × max_similarity(chunk, already_selected)
```

- `λ = 1.0` - pure relevance (top-K by score)
- `λ = 0.5` - balanced (default)
- `λ = 0.0` - pure diversity (maximize distance from selected chunks)

### What's the time complexity?

Distance matrix computation is O(N² × D) where N = number of chunks and D = embedding dimension. The merge loop is O(N³) worst case. For typical RAG inputs (N=20-50, D=1536), the full pipeline completes in ~12ms. For larger inputs (N=1000+), the K-Means path with parallel workers is available.

### How does compression work without an LLM?

Three rule-based strategies, chainable via a pipeline:

1. **Extractive** - Scores sentences by position, length, and keyword signals. Keeps the top sentences within a token budget.
2. **Placeholder** - Detects JSON, XML, and tables. Replaces them with structural summaries (e.g., `[JSON object with 12 keys: id, name, ...]`).
3. **Pruner** - Removes filler phrases ("as mentioned earlier", "basically", "it is important to note that") and intensifiers.

No API calls needed.

---

## Integration

### How does Distill work with LangChain?

Three integration paths, from simplest to deepest:

**1. MCP (works today):** Distill ships an MCP server (`distill mcp`). LangChain supports MCP via [`langchain-mcp-adapters`](https://github.com/langchain-ai/langchain-mcp-adapters). Distill's tools (`deduplicate_chunks`, `retrieve_deduplicated`, `analyze_redundancy`) become LangChain tools automatically.

```python
from langchain_mcp_adapters.client import MultiServerMCPClient
from langchain.agents import create_agent

client = MultiServerMCPClient({
    "distill": {
        "command": "distill",
        "args": ["mcp"],
        "transport": "stdio",
    }
})
tools = await client.get_tools()
agent = create_agent("openai:gpt-4.1", tools)
```

**2. HTTP API (works today):** Call `POST /v1/dedupe` as a post-processing step on retrieval results.

```python
import httpx

def deduplicate(docs, threshold=0.15):
    chunks = [{"id": str(i), "text": doc.page_content} for i, doc in enumerate(docs)]
    resp = httpx.post("https://distill-api-4u92.onrender.com/v1/dedupe", json={
        "chunks": chunks, "threshold": threshold
    })
    kept = {c["id"] for c in resp.json()["chunks"]}
    return [doc for i, doc in enumerate(docs) if str(i) in kept]

raw_docs = retriever.invoke("query")  # Over-fetch 20 results
clean_docs = deduplicate(raw_docs)    # -> ~8 unique results
```

**3. Python SDK (planned - [#5](https://github.com/Siddhant-K-code/distill/issues/5)):** A `DistillRetriever` that wraps any LangChain retriever with automatic dedup.

### Does it work with LlamaIndex, CrewAI, AutoGen, etc.?

Yes. The HTTP API is framework-agnostic. MCP works with any MCP-compatible client. The planned Python SDK ([#5](https://github.com/Siddhant-K-code/distill/issues/5)) will include a LlamaIndex `NodePostprocessor`.

### How is this different from LangChain's built-in MMR retriever?

LangChain's `search_type="mmr"` applies MMR at the vector DB level - a single re-ranking step. Distill runs a multi-stage pipeline: cache lookup, agglomerative clustering (groups similar chunks), representative selection (picks the best from each group), compression (reduces token count), then MMR (diversity re-ranking). The clustering step is the key difference - it understands group structure, not just pairwise similarity.

### What MCP tools does Distill expose?

The base MCP server exposes `deduplicate_context` and `analyze_redundancy`. With `--memory`, it adds `store_memory`, `recall_memory`, `forget_memory`, `memory_stats`. With `--session`, it adds `create_session`, `push_session`, `session_context`, `delete_session`. Enable both with `distill mcp --memory --session`.

### Can I use Distill with local models (Ollama, vLLM)?

The dedup pipeline itself doesn't call any LLM - it's pure math (cosine distance, clustering). The only external dependency is for embedding generation when you send text without pre-computed embeddings. Multi-provider embedding support (Ollama, Azure, Cohere, HuggingFace) is planned in [#33](https://github.com/Siddhant-K-code/distill/issues/33).

---

## Performance & Cost

### What's the latency overhead?

~12ms total for the pipeline: distance matrix ~2ms, clustering ~6ms, selection <1ms, MMR ~3ms. Embedding generation adds more if needed (depends on OpenAI API latency, typically 100-300ms for a batch). If embeddings are pre-computed, it's just the 12ms.

### What's the cost?

If chunks already have embeddings (from your vector DB): **$0**. If text-only chunks are sent, Distill uses `text-embedding-3-small` at $0.02 per 1M tokens. A typical 20-chunk request with ~100 tokens each = 2,000 tokens = $0.00004.

### Does it scale to thousands of chunks?

The agglomerative clustering is O(N²) for the distance matrix. For N=50, this is trivial (~2ms). For N=1,000, it's still fast (~100ms). For N=10,000+, the K-Means path (`pkg/dedup/`) with parallel workers is available. A batch API is planned in [#11](https://github.com/Siddhant-K-code/distill/issues/11).

### What if chunks don't have embeddings?

If you send text-only chunks to the API, Distill calls OpenAI's `text-embedding-3-small` to generate embeddings on the fly. Set `OPENAI_API_KEY` to enable this. If you send chunks with pre-computed embeddings (e.g., from your vector DB retrieval), no OpenAI call is needed.

---

## Deployment

### How do I self-host Distill?

Three options:

```bash
# Binary
distill api --port 8080

# Docker
docker run -p 8080:8080 -e OPENAI_API_KEY=xxx ghcr.io/siddhant-k-code/distill

# Build from source
go build -o distill . && ./distill api
```

### How do I protect my self-hosted instance?

Set `DISTILL_API_KEYS` with comma-separated API keys. Clients must include `Authorization: Bearer <key>` in requests.

```bash
export DISTILL_API_KEYS="key1,key2,key3"
distill api --port 8080
```

### What observability is available?

- **Prometheus metrics** at `/metrics` - request counts, latency histograms, chunk reduction ratios, cluster counts
- **OpenTelemetry tracing** - per-stage spans (embedding, clustering, selection, MMR) with W3C Trace Context propagation
- **Grafana dashboard** - pre-built template in `grafana/`

---

## Context & Positioning

### Why should I use this instead of just increasing my context window?

Larger context windows don't solve redundancy. If you stuff 50 chunks into a 128K window and 20 say the same thing, the model still processes all of them. This wastes tokens, increases latency, and can confuse the model. Distill ensures the model sees unique, diverse chunks instead of overlapping ones.

### Is Distill open source?

Yes, AGPL-3.0. The full pipeline, CLI, API server, MCP server, and all algorithms are open source. Commercial licensing is available for closed-source usage - contact siddhantkhare2694@gmail.com.

### What's on the roadmap?

**Shipped:**
- **Context Memory** - Persistent deduplicated memory across sessions with hierarchical decay ([#29](https://github.com/Siddhant-K-code/distill/issues/29))
- **Session Management** - Token-budgeted context windows with compression and eviction ([#31](https://github.com/Siddhant-K-code/distill/issues/31))

**Upcoming:**
1. **Code Intelligence** - Dependency graphs, co-change patterns, blast radius analysis ([#30](https://github.com/Siddhant-K-code/distill/issues/30), [#32](https://github.com/Siddhant-K-code/distill/issues/32))
2. **Platform** - Python SDK, multi-provider embeddings, batch API ([#5](https://github.com/Siddhant-K-code/distill/issues/5), [#33](https://github.com/Siddhant-K-code/distill/issues/33), [#11](https://github.com/Siddhant-K-code/distill/issues/11))
