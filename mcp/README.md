# GoVectorSync MCP Integration

GoVectorSync exposes semantic deduplication as an MCP (Model Context Protocol) server, allowing AI assistants to deduplicate context directly.

## The Problem

When AI assistants assemble context from multiple sources (code, docs, memory, tool outputs), 30-40% is typically redundant. Same information from different sources wastes tokens and confuses the model.

## How GoVectorSync Helps

GoVectorSync sits between your context sources and the LLM:

```
RAG Results → GoVectorSync → Deduplicated Context → LLM
```

It clusters semantically similar chunks, picks the best representative from each cluster, and applies MMR for diversity.

## Quick Start

### Local (stdio) - Claude Desktop, Cursor, Amp

```bash
# Build
go build -o govs .

# Start MCP server
./govs mcp
```

### Remote (HTTP) - Hosted deployment

```bash
# Start HTTP server
./govs mcp --transport http --port 8081

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

## Resources

### `govectorsync://system-prompt`

System prompt that guides AI assistants to use deduplication effectively. Host applications can include this in context automatically.

### `govectorsync://config`

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

**Local (stdio):**
```json
{
  "mcpServers": {
    "govectorsync": {
      "command": "/path/to/govs",
      "args": ["mcp"]
    }
  }
}
```

**Remote (HTTP):**
```json
{
  "mcpServers": {
    "govectorsync": {
      "url": "https://govectorsync-mcp.fly.dev/mcp"
    }
  }
}
```

**With vector DB backend:**
```json
{
  "mcpServers": {
    "govectorsync": {
      "command": "/path/to/govs",
      "args": ["mcp", "--backend", "pinecone", "--index", "my-index"],
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

Your MCP endpoint: `https://govectorsync-mcp.fly.dev/mcp`

### Docker

```bash
# Build
docker build -f Dockerfile.mcp -t govectorsync-mcp .

# Run
docker run -p 8081:8081 \
  -e OPENAI_API_KEY=xxx \
  govectorsync-mcp
```

### Railway / Render / Other

Use the Dockerfile.mcp and set:
- Port: 8081
- Command: `./govs mcp --transport http --port 8081`

## Integration Patterns

### Pattern 1: Automatic Context Optimization

The AI reads the system prompt resource and automatically deduplicates when it detects multiple chunks:

```
1. Host app includes govectorsync://system-prompt in context
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

### Pattern 4: Direct Vector DB Query

If backend is configured, query with automatic deduplication:

```
User: "Search for authentication docs"
AI: [calls retrieve_deduplicated with query]
AI: "Found 8 diverse results from 50 retrieved"
```

## When Does the AI Call These Tools?

The AI decides to call tools based on:

1. **Tool descriptions** - We've written action-oriented descriptions that explain when to use each tool
2. **System prompt** - The `govectorsync://system-prompt` resource guides the AI to check for redundancy
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

| Aspect | MCP Server | HTTP Proxy (`govs serve`) |
|--------|------------|---------------------------|
| Invocation | AI decides when to call | Automatic on every query |
| Integration | MCP-compatible clients | Any HTTP client |
| Use case | AI assistant workflows | RAG pipeline middleware |
| Control | AI-driven | Application-driven |

**Recommendation:** Use HTTP proxy for automatic deduplication in RAG pipelines. Use MCP for AI assistant integrations where the AI needs to decide when to deduplicate.
