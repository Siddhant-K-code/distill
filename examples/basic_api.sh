#!/bin/bash
# Basic example: Deduplicate chunks using the standalone API

# Start the server (in another terminal):
# export OPENAI_API_KEY="your-key"
# distill api --port 8080

# Send chunks for deduplication
curl -X POST http://localhost:8080/v1/dedupe \
  -H "Content-Type: application/json" \
  -d '{
    "chunks": [
      {"id": "1", "text": "React is a JavaScript library for building user interfaces."},
      {"id": "2", "text": "React.js is a JS library for building UIs."},
      {"id": "3", "text": "Vue is a progressive JavaScript framework."},
      {"id": "4", "text": "Vue.js is a progressive framework for building UIs."},
      {"id": "5", "text": "Angular is a platform for building web applications."}
    ],
    "threshold": 0.15,
    "lambda": 0.5
  }' | jq .
