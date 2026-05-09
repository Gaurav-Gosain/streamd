#!/usr/bin/env bash
# Emit SSE chunks with reasoning_content (thinking tokens) followed by
# the final answer. Matches the format used by Qwen / DeepSeek and
# OpenAI-compatible APIs that expose chain-of-thought.

set -e

emit_think() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"reasoning_content":%s}}],"model":"qwen3"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.03
}

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"qwen3"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.03
}

think='The user asks for the capital of France. This is a simple
factual question. The capital of France is Paris. I should
give a short, direct answer.'

answer='The capital of France is **Paris**.'

while IFS= read -r line; do
  for word in $line; do
    emit_think "$word "
  done
  emit_think $'\n'
done <<< "$think"

while IFS= read -r line; do
  for word in $line; do
    emit "$word "
  done
  emit $'\n'
done <<< "$answer"

printf 'data: [DONE]\n\n'
