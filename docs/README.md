# Distill Documentation

## Guides

- [Getting Started](guides/getting-started.md) — Install, configure, and run your first dedup
- [Memory](guides/memory.md) — Persistent context memory across sessions
- [Sessions](guides/sessions.md) — Token-budgeted context windows
- [MCP Integration](guides/mcp.md) — Use Distill with Claude Desktop, Cursor, and other MCP clients
- [Deployment](guides/deployment.md) — Docker, binary, and cloud deployment
- [Handoff Pilot Kit](handoff-pilot-kit.md): Run one review-only retrospective trial
- [Handoff internal dogfood](handoff-dogfood-pr-112.md): PR #112 baseline result

## Reference

- [API Reference](reference/api.md) — All REST endpoints
- [Configuration](reference/configuration.md) — Config file, environment variables, CLI flags
- [Distill Lock v0](distill-lock-v0.md) — Deterministic context lock/build/verify contract
- [Handoff v0 alpha](handoff-v0.md) — Experimental review-only decision-to-docs bundles
- [OpenAPI Spec](../openapi.yaml) — Machine-readable API specification

## Examples

- [LangChain Integration](examples/langchain.md)
- [RAG Pipeline](examples/rag-pipeline.md)

## Research

- [Context Is a Build Artifact](../research/context-is-a-build-artifact/):
  preregistered decision-reliability design and local v2 aggregate result
- [Handoff PR #112 repeatability](../research/handoff-repeatability-pr-112/):
  frozen three-run internal repeatability result
- [Handoff PR #112 temporal-state guard](../research/handoff-temporal-state-guard-pr-112/):
  preregistered three-run prompt ablation
