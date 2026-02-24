#!/bin/bash
# Example: Session-based context window management via the API
#
# Start the server (in another terminal):
#   distill api --port 8080 --session
#
# Sessions track context for long-running agent tasks with a token budget.
# Entries are deduplicated on push, compressed as they age, and evicted
# when the budget is exceeded.

BASE="http://localhost:8080"

echo "=== Create session ==="
curl -s -X POST "$BASE/v1/session/create" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "demo-task",
    "max_tokens": 50000
  }' | jq .

echo ""
echo "=== Push context entries ==="
curl -s -X POST "$BASE/v1/session/push" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "demo-task",
    "entries": [
      {
        "role": "user",
        "content": "Fix the JWT validation bug in the auth service",
        "importance": 1.0
      },
      {
        "role": "tool",
        "content": "File: auth/jwt.go\n\npackage auth\n\nimport (\n\t\"crypto/rsa\"\n\t\"time\"\n)\n\nfunc ValidateToken(token string, key *rsa.PublicKey) error {\n\t// BUG: not checking expiry\n\treturn nil\n}",
        "source": "file_read",
        "importance": 0.8
      },
      {
        "role": "tool",
        "content": "File: auth/jwt_test.go\n\npackage auth\n\nimport \"testing\"\n\nfunc TestValidateToken(t *testing.T) {\n\t// No expiry test\n}",
        "source": "file_read",
        "importance": 0.6
      },
      {
        "role": "assistant",
        "content": "The ValidateToken function is missing expiry checks. I will add time.Now().After(claims.ExpiresAt) validation.",
        "importance": 0.9
      }
    ]
  }' | jq .

echo ""
echo "=== Read context window ==="
curl -s -X POST "$BASE/v1/session/context" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "demo-task"
  }' | jq .

echo ""
echo "=== Read only tool entries ==="
curl -s -X POST "$BASE/v1/session/context" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "demo-task",
    "role": "tool"
  }' | jq .

echo ""
echo "=== Get session metadata ==="
curl -s "$BASE/v1/session/get?session_id=demo-task" | jq .

echo ""
echo "=== Clean up ==="
curl -s -X DELETE "$BASE/v1/session/delete" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "demo-task"}' | jq .
