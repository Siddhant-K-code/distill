#!/bin/bash

set -e

rm -rf completions
mkdir -p completions

# Generate shell completion scripts for all supported shells
for sh in bash zsh fish powershell; do
  go run . completion "$sh" > "completions/distill.$sh"
done

