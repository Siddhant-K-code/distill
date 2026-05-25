# MCP Integration

Distill runs as an [MCP](https://modelcontextprotocol.io/) server, exposing its tools to Claude Desktop, Cursor, and other MCP-compatible clients.

## Start the MCP server

```bash
# Basic dedup tools
distill mcp

# With memory and sessions
distill mcp --memory --session

# With Ollama embeddings
distill mcp --embedding-provider ollama --embedding-model nomic-embed-text
```

## Configure Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "distill": {
      "command": "distill",
      "args": ["mcp", "--memory", "--session"],
      "env": {
        "OPENAI_API_KEY": "sk-..."
      }
    }
  }
}
```

## Configure Cursor

Add to `.cursor/mcp.json` in your project:

```json
{
  "mcpServers": {
    "distill": {
      "command": "distill",
      "args": ["mcp", "--memory"]
    }
  }
}
```

## Available tools

### Dedup tools

| Tool | Description |
|------|-------------|
| `deduplicate_chunks` | Deduplicate a list of text chunks |
| `retrieve_deduplicated` | Query vector DB with dedup (requires `--retriever`) |
| `analyze_redundancy` | Analyze redundancy in a set of chunks |

### Memory tools (requires `--memory`)

| Tool | Description |
|------|-------------|
| `store_memory` | Store a memory entry with optional sensitivity tagging |
| `recall_memory` | Recall memories by query with relevance ranking |
| `forget_memory` | Remove memories by ID, tag, or age |
| `memory_expire` | Mark memories as expired |
| `memory_supersede` | Replace a memory with a newer version |
| `memory_stats` | Get memory store statistics |

### Session tools (requires `--session`)

| Tool | Description |
|------|-------------|
| `create_session` | Create a token-budgeted context window |
| `push_session` | Add entries to a session |
| `session_context` | Read the current context window |
| `delete_session` | Delete a session |

## Example usage in Claude

Once configured, Claude can use Distill tools directly:

> "Store this architecture decision: we're using JWT with RS256 for auth"

Claude will call `store_memory` with the text, and Distill will deduplicate, classify sensitivity, and persist it.

> "What do we know about authentication?"

Claude will call `recall_memory` with the query, and Distill will return ranked, deduplicated memories with sensitivity metadata.
