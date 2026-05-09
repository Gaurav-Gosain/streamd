#!/usr/bin/env bash
# Emit OpenAI Chat Completions SSE chunks with a short delay between
# tokens so the demo looks like a real streaming response.

set -e

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"demo"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.03
}

text='# Quicksort

Quicksort is a **divide-and-conquer** sorting algorithm with average
`O(n log n)` time complexity.

## How it works

1. Pick a *pivot* element from the array.
2. **Partition** the rest into two groups: less than pivot, greater than pivot.
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

That is the whole algorithm in 15 lines of Go.'

# Stream word-by-word for a realistic streaming feel.
while IFS= read -r line; do
  for word in $line; do
    emit "$word "
  done
  emit $'\n'
done <<< "$text"

printf 'data: [DONE]\n\n'
