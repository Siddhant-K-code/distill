# Distill MCP Integration

Distill exposes semantic deduplication as an MCP (Model Context Protocol) server, allowing AI assistants to deduplicate context directly.

## The Problem

When AI assistants assemble context from multiple sources (code, docs, memory, tool outputs), 30-40% is typically redundant. Same information from different sources wastes tokens and confuses the model.

## How Distill Helps

Distill sits between your context sources and the LLM:

```
RAG Results → Distill → Deduplicated Context → LLM
```

It clusters semantically similar chunks, picks the best representative from each cluster, and applies MMR for diversity.

## Quick Start

### Local (stdio) - Claude Desktop, Cursor, Amp

```bash
# Build
go build -o distill .

# Start MCP server (dedup only)
./distill mcp

# With memory and sessions enabled
./distill mcp --memory --session
```

### Remote (HTTP) - Hosted deployment

```bash
# Start HTTP server
./distill mcp --transport http --port 8081

# With all features
./distill mcp --transport http --port 8081 --memory --session

# Or deploy to Fly.io
fly deploy -c fly.mcp.toml
```

## Tools

### `deduplicate_chunks`

Remove redundant information from RAG chunks. Works without any external dependencies.

**When to use:** Whenever you have multiple chunks from retrieval that might overlap.

```json
{
  "chunks": [
    {"id": "1", "text": "How to install Python", "embedding": [0.1, 0.2, ...]},
    {"id": "2", "text": "Python installation guide", "embedding": [0.11, 0.21, ...]}
  ],
  "target_k": 5,
  "threshold": 0.15,
  "lambda": 0.5
}
```

### `retrieve_deduplicated`

Query a vector database with automatic deduplication. Requires `--backend` flag.

```json
{
  "query": "authentication best practices",
  "target_k": 5,
  "over_fetch_k": 25
}
```

### `analyze_redundancy`

Analyze chunks for redundancy without removing any. Use to understand overlap before deduplicating.

### `store_memory` (requires `--memory`)

Store context that should persist across sessions. Memories are deduplicated on write.

```json
{
  "text": "Auth service uses JWT with RS256 signing",
  "tags": ["auth", "jwt"],
  "source": "code_review"
}
```

### `recall_memory` (requires `--memory`)

Recall relevant memories by semantic similarity + recency.

```json
{
  "query": "How does authentication work?",
  "max_results": 5,
  "tags": ["auth"]
}
```

### `forget_memory` (requires `--memory`)

Remove memories by tag or age.

### `memory_stats` (requires `--memory`)

Get memory store statistics (total count, by decay level, by source).

### `create_session` (requires `--session`)

Create a token-budgeted context window for a task.

```json
{
  "session_id": "fix-auth-bug",
  "max_tokens": 128000
}
```

### `push_session` (requires `--session`)

Push context entries to a session. Entries are deduplicated and the token budget is enforced via compression and eviction.

```json
{
  "session_id": "fix-auth-bug",
  "content": "File: auth/jwt.go\n...",
  "role": "tool",
  "source": "file_read",
  "importance": 0.8
}
```

### `session_context` (requires `--session`)

Read the current context window. Returns entries in push order with compression levels and token counts.

```json
{
  "session_id": "fix-auth-bug",
  "max_tokens": 50000
}
```

### `delete_session` (requires `--session`)

Delete a session and all its entries.

## Resources

### `distill://system-prompt`

System prompt that guides AI assistants to use deduplication effectively. Host applications can include this in context automatically.

### `distill://config`

Current configuration and defaults (JSON).

## Prompts

### `optimize-rag-context`

Template for optimizing RAG context before answering a question:

```
Arguments:
  - question: The user's question
  - chunks_json: JSON array of chunks with embeddings
```

## Configuration

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

**Local (stdio) - dedup only:**
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

**With memory and sessions:**
```json
{
  "mcpServers": {
    "distill": {
      "command": "/path/to/distill",
      "args": ["mcp", "--memory", "--session"],
      "env": {
        "OPENAI_API_KEY": "your-openai-key"
      }
    }
  }
}
```

**Remote (HTTP):**
```json
{
  "mcpServers": {
    "distill": {
      "url": "https://distill-mcp.fly.dev/mcp"
    }
  }
}
```

**With vector DB backend:**
```json
{
  "mcpServers": {
    "distill": {
      "command": "/path/to/distill",
      "args": ["mcp", "--backend", "pinecone", "--index", "my-index", "--memory", "--session"],
      "env": {
        "PINECONE_API_KEY": "your-api-key",
        "OPENAI_API_KEY": "your-openai-key"
      }
    }
  }
}
```

### Cursor

Add to Cursor's MCP settings (same format as Claude Desktop).

### Amp (Sourcegraph)

Add to Amp's MCP configuration (same format as Claude Desktop).

## Deployment

### Fly.io

```bash
# First time
fly launch -c fly.mcp.toml

# Set secrets
fly secrets set OPENAI_API_KEY=xxx -c fly.mcp.toml
fly secrets set PINECONE_API_KEY=xxx -c fly.mcp.toml

# Deploy
fly deploy -c fly.mcp.toml
```

Your MCP endpoint: `https://distill-mcp.fly.dev/mcp`

### Docker

```bash
# Build
docker build -f Dockerfile.mcp -t distill-mcp .

# Run
docker run -p 8081:8081 \
  -e OPENAI_API_KEY=xxx \
  distill-mcp
```

### Railway / Render / Other

Use the Dockerfile.mcp and set:
- Port: 8081
- Command: `./distill mcp --transport http --port 8081`

## Integration Patterns

### Pattern 1: Automatic Context Optimization

The AI reads the system prompt resource and automatically deduplicates when it detects multiple chunks:

```
1. Host app includes distill://system-prompt in context
2. User asks a question
3. RAG retrieves chunks
4. AI recognizes overlap, calls deduplicate_chunks
5. AI answers using deduplicated context
```

### Pattern 2: Explicit Deduplication

User explicitly requests deduplication:

```
User: "I have these 10 chunks from my search. Can you deduplicate them?"
AI: [calls deduplicate_chunks]
AI: "Reduced to 4 unique chunks (60% redundancy removed)"
```

### Pattern 3: Analysis First

Analyze before deciding to deduplicate:

```
User: "How much overlap is in my context?"
AI: [calls analyze_redundancy]
AI: "Found 40% redundancy across 3 clusters. Want me to deduplicate?"
```

### Pattern 4: Session-Based Context Tracking

Track context across a multi-step task:

```
1. AI creates a session: create_session("fix-auth-bug", 128000)
2. AI reads files: push_session(role="tool", content=file, source="file_read")
3. AI reads tests: push_session(role="tool", content=tests, source="file_read")
4. Budget exceeded → oldest low-importance entries compressed automatically
5. AI reads context: session_context() → deduplicated, budget-aware window
6. Task done: delete_session()
```

### Pattern 5: Cross-Session Memory

Persist knowledge that should survive across sessions:

```
1. AI discovers a pattern: store_memory("Auth uses JWT with RS256", tags=["auth"])
2. Next session, different task: recall_memory("How does auth work?")
3. AI gets relevant memories without re-reading files
```

### Pattern 6: Direct Vector DB Query

If backend is configured, query with automatic deduplication:

```
User: "Search for authentication docs"
AI: [calls retrieve_deduplicated with query]
AI: "Found 8 diverse results from 50 retrieved"
```

## When Does the AI Call These Tools?

The AI decides to call tools based on:

1. **Tool descriptions** - We've written action-oriented descriptions that explain when to use each tool
2. **System prompt** - The `distill://system-prompt` resource guides the AI to check for redundancy
3. **User requests** - Explicit requests like "deduplicate these" or "remove redundancy"
4. **Context patterns** - When the AI sees multiple similar chunks, it may recognize the need

**To maximize tool usage:**
- Include the system prompt resource in your host application
- Use the `optimize-rag-context` prompt template
- Mention "deduplicate" or "redundancy" in user queries

## Tuning Parameters

| Parameter | Default | When to Adjust |
|-----------|---------|----------------|
| `threshold` | 0.15 | Lower (0.1) for code, higher (0.2) for prose |
| `lambda` | 0.5 | Higher for relevance, lower for diversity |
| `target_k` | 8 | Based on your context window budget |
| `over_fetch_k` | 50 | 3-5x target_k for best results |

## Performance

- Clustering: ~6ms for 50 chunks
- MMR re-ranking: ~3ms
- Total overhead: ~12ms

## Comparison: MCP vs HTTP Proxy

| Aspect | MCP Server | HTTP Proxy (`distill serve`) |
|--------|------------|------------------------------|
| Invocation | AI decides when to call | Automatic on every query |
| Integration | MCP-compatible clients | Any HTTP client |
| Use case | AI assistant workflows | RAG pipeline middleware |
| Control | AI-driven | Application-driven |

**Recommendation:** Use HTTP proxy for automatic deduplication in RAG pipelines. Use MCP for AI assistant integrations where the AI needs to decide when to deduplicate.
