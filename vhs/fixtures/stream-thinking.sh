#!/usr/bin/env bash
# Emit SSE chunks with reasoning_content (thinking tokens) followed by
# the final answer. Matches the format used by Qwen / DeepSeek and
# OpenAI-compatible APIs that expose chain-of-thought.

set -e

emit_think() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"reasoning_content":%s}}],"model":"qwen3"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.018
}

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"qwen3"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.025
}

# Single-paragraph thinking content keeps the blockquote rendering
# clean across terminal widths.
think='User asks the capital of France. Simple factual question. The answer is Paris. I will give a short, direct response.'
answer='The capital of France is **Paris**.'

stream() {
  local fn="$1" text="$2" chunk=3
  local i=0 len=${#text}
  while [ $i -lt $len ]; do
    "$fn" "${text:$i:$chunk}"
    i=$((i + chunk))
  done
}

stream emit_think "$think"
stream emit "$answer"

printf 'data: [DONE]\n\n'
