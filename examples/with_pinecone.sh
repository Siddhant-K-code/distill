#!/bin/bash
# Example: Query Pinecone with automatic deduplication

# Start the server (in another terminal):
# export OPENAI_API_KEY="your-key"
# export PINECONE_API_KEY="your-key"
# distill serve --index my-index --port 8080

# Query with deduplication
curl -X POST http://localhost:8080/v1/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "query": "How do I deploy a Next.js application?",
    "target_k": 8,
    "over_fetch_k": 50,
    "threshold": 0.15,
    "lambda": 0.5
  }' | jq .
