# RAG Pipeline with Distill

Use Distill's dedup + memory pipeline to build a retrieval-augmented generation system that avoids redundant context and remembers past interactions.

## Architecture

```
Documents → Distill (dedup + embed) → Memory Store
                                          ↓
User Query → Distill (recall) → Ranked Context → LLM → Response
                                                    ↓
                                          Distill (store answer)
```

## Ingest documents

Deduplicate and store document chunks:

```python
import requests

DISTILL = "http://localhost:8080"

def ingest(chunks: list[str], source: str):
    """Dedup chunks, then store unique ones as memories."""
    # Deduplicate
    resp = requests.post(f"{DISTILL}/v1/dedupe", json={
        "chunks": chunks,
    })
    resp.raise_for_status()
    unique = resp.json()["unique_chunks"]

    # Store each unique chunk
    for chunk in unique:
        requests.post(f"{DISTILL}/v1/memory/store", json={
            "content": chunk,
            "agent_id": "rag-pipeline",
            "tags": ["document", source],
            "auto_classify": True,
        }).raise_for_status()

    print(f"Stored {len(unique)}/{len(chunks)} chunks from {source}")
```

## Query with context

```python
def query(question: str, llm_fn) -> str:
    """Recall relevant context and generate an answer."""
    # Recall from memory
    resp = requests.post(f"{DISTILL}/v1/memory/recall", json={
        "query": question,
        "agent_id": "rag-pipeline",
        "top_k": 10,
        "min_relevance": 0.3,
        "boost_tags": ["document"],
    })
    resp.raise_for_status()
    memories = resp.json()["memories"]

    # Build prompt
    context = "\n---\n".join(m["content"] for m in memories)
    prompt = f"Context:\n{context}\n\nQuestion: {question}\nAnswer:"

    answer = llm_fn(prompt)

    # Store the Q&A pair for future recall
    requests.post(f"{DISTILL}/v1/memory/store", json={
        "content": f"Q: {question}\nA: {answer}",
        "agent_id": "rag-pipeline",
        "tags": ["qa"],
    }).raise_for_status()

    return answer
```

## Handle stale knowledge

When documents are updated, supersede old memories:

```python
def update_document(old_memory_id: str, new_content: str):
    """Replace outdated memory with new version."""
    # Store new version
    resp = requests.post(f"{DISTILL}/v1/memory/store", json={
        "content": new_content,
        "agent_id": "rag-pipeline",
        "tags": ["document"],
    })
    resp.raise_for_status()
    new_id = resp.json()["id"]

    # Supersede old version
    requests.post(f"{DISTILL}/v1/memory/supersede", json={
        "id": old_memory_id,
        "new_id": new_id,
    }).raise_for_status()
```

## Session-based context window

For multi-turn conversations, use sessions to manage token budgets:

```python
def create_session():
    resp = requests.post(f"{DISTILL}/v1/session/create", json={
        "max_tokens": 4000,
        "strategy": "sliding_window",
    })
    return resp.json()["session_id"]

def add_to_session(session_id: str, role: str, content: str):
    requests.post(f"{DISTILL}/v1/session/push", json={
        "session_id": session_id,
        "entries": [{"role": role, "content": content}],
    }).raise_for_status()

def get_context(session_id: str) -> list[dict]:
    resp = requests.post(f"{DISTILL}/v1/session/context", json={
        "session_id": session_id,
    })
    return resp.json()["entries"]
```

## Full example

```python
from openai import OpenAI

client = OpenAI()

def llm(prompt: str) -> str:
    resp = client.chat.completions.create(
        model="gpt-4o",
        messages=[{"role": "user", "content": prompt}],
    )
    return resp.choices[0].message.content

# Ingest
chunks = [
    "Distill deduplicates context before sending to LLMs.",
    "Memory entries decay over time based on access patterns.",
    "Sensitivity classification detects PII automatically.",
]
ingest(chunks, source="distill-docs")

# Query
answer = query("How does Distill handle sensitive data?", llm)
print(answer)
```
