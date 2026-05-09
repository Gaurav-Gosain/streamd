#!/usr/bin/env bash
# Emit a short SSE response that ends with usage info, so streamd --info
# has something to display.

set -e

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"gpt-4o-mini"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.04
}

text='**Streamd** auto-detects the input format and renders streaming
markdown in the terminal.'

while IFS= read -r line; do
  for word in $line; do
    emit "$word "
  done
  emit $'\n'
done <<< "$text"

# Final usage chunk.
printf 'data: {"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":42,"completion_tokens":18,"total_tokens":60},"model":"gpt-4o-mini"}\n\n'
printf 'data: [DONE]\n\n'
