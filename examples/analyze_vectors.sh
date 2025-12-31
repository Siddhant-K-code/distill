#!/bin/bash
# Example: Analyze a JSONL file for duplicates before uploading

# Create sample vectors file
cat > /tmp/vectors.jsonl << 'EOF'
{"id": "doc1", "values": [0.1, 0.2, 0.3, 0.4], "metadata": {"source": "docs"}}
{"id": "doc2", "values": [0.11, 0.21, 0.31, 0.41], "metadata": {"source": "docs"}}
{"id": "doc3", "values": [0.9, 0.8, 0.7, 0.6], "metadata": {"source": "code"}}
EOF

# Analyze for duplicates
distill analyze --file /tmp/vectors.jsonl --threshold 0.05

# Output:
# Total vectors analyzed:  3
# Unique vectors:          2
# Duplicates found:        1
# Potential savings:       33.3%
