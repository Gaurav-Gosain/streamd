#!/usr/bin/env bash
# Emit OpenAI Chat Completions SSE chunks. Streams by small character
# chunks so all whitespace (including code indentation and newlines)
# is preserved exactly as written.

set -e

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"demo"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.018
}

read -r -d '' text <<'EOF' || true
# Quicksort

Quicksort is a **divide-and-conquer** sorting algorithm with average
`O(n log n)` time complexity.

## How it works

1. Pick a *pivot* element from the array.
2. **Partition** the rest into two groups.
3. Recursively sort each partition.

## Implementation

```go
func quicksort(arr []int) []int {
    if len(arr) <= 1 {
        return arr
    }
    pivot := arr[len(arr)/2]
    var less, equal, greater []int
    for _, n := range arr {
        switch {
        case n < pivot:
            less = append(less, n)
        case n == pivot:
            equal = append(equal, n)
        default:
            greater = append(greater, n)
        }
    }
    return append(append(quicksort(less), equal...), quicksort(greater)...)
}
```

That is the whole algorithm in 15 lines of Go.
EOF

# Stream in 4-char chunks so the demo shows incremental rendering
# without dropping any whitespace.
chunk=4
len=${#text}
i=0
while [ $i -lt $len ]; do
  emit "${text:$i:$chunk}"
  i=$((i + chunk))
done

printf 'data: [DONE]\n\n'
