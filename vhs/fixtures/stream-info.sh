#!/usr/bin/env bash
# Emit a short SSE response that ends with usage info, so streamd --info
# has something to display.

set -e

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"gpt-4o-mini"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.025
}

text='**streamd** auto-detects the input format and renders streaming markdown directly in your terminal.'

chunk=3
len=${#text}
i=0
while [ $i -lt $len ]; do
  emit "${text:$i:$chunk}"
  i=$((i + chunk))
done

# Final usage chunk.
printf 'data: {"choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":42,"completion_tokens":18,"total_tokens":60},"model":"gpt-4o-mini"}\n\n'
printf 'data: [DONE]\n\n'
