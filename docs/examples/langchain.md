# LangChain Integration

Use Distill as a persistent memory layer for LangChain agents.

## Setup

```bash
pip install langchain requests
```

## Memory wrapper

```python
import requests
from langchain.memory import BaseMemory

class DistillMemory(BaseMemory):
    """LangChain memory backed by Distill's memory API."""

    base_url: str = "http://localhost:8080"
    api_key: str | None = None
    agent_id: str = "langchain-agent"
    memory_key: str = "history"

    @property
    def memory_variables(self) -> list[str]:
        return [self.memory_key]

    def _headers(self) -> dict:
        h = {"Content-Type": "application/json"}
        if self.api_key:
            h["Authorization"] = f"Bearer {self.api_key}"
        return h

    def load_memory_variables(self, inputs: dict) -> dict:
        query = inputs.get("input", "")
        resp = requests.post(
            f"{self.base_url}/v1/memory/recall",
            headers=self._headers(),
            json={
                "query": query,
                "agent_id": self.agent_id,
                "top_k": 5,
            },
        )
        resp.raise_for_status()
        memories = resp.json().get("memories", [])
        text = "\n".join(m["content"] for m in memories)
        return {self.memory_key: text}

    def save_context(self, inputs: dict, outputs: dict) -> None:
        content = f"User: {inputs.get('input', '')}\nAssistant: {outputs.get('output', '')}"
        requests.post(
            f"{self.base_url}/v1/memory/store",
            headers=self._headers(),
            json={
                "content": content,
                "agent_id": self.agent_id,
                "tags": ["conversation"],
                "auto_classify": True,
            },
        ).raise_for_status()

    def clear(self) -> None:
        requests.post(
            f"{self.base_url}/v1/memory/forget",
            headers=self._headers(),
            json={"agent_id": self.agent_id},
        ).raise_for_status()
```

## Usage with an agent

```python
from langchain.chat_models import ChatOpenAI
from langchain.chains import ConversationChain

memory = DistillMemory(
    base_url="http://localhost:8080",
    agent_id="my-agent",
)

chain = ConversationChain(
    llm=ChatOpenAI(model="gpt-4o"),
    memory=memory,
)

# Memories persist across restarts
response = chain.predict(input="What did we discuss yesterday?")
```

## Why use Distill instead of LangChain's built-in memory?

- **Persistence** — survives process restarts, stored in SQLite
- **Deduplication** — repeated context is stored once
- **Sensitivity tagging** — PII and credentials are flagged automatically
- **Decay** — old memories lose relevance over time
- **Conflict detection** — contradictory memories are surfaced
