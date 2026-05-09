#!/usr/bin/env bash
# Long-form response that overflows the viewport; used to demo
# alt-screen scrolling. Streams by character chunks so indentation
# in code blocks is preserved.

set -e

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"demo"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.012
}

read -r -d '' text <<'EOF' || true
# Reversing a Linked List in Go

Two classic approaches: **iterative** and **recursive**.

## Iterative approach

Walk the list once, flipping each `next` pointer.

```go
type Node struct {
    Val  int
    Next *Node
}

func reverse(head *Node) *Node {
    var prev *Node
    curr := head
    for curr != nil {
        next := curr.Next
        curr.Next = prev
        prev = curr
        curr = next
    }
    return prev
}
```

Time complexity: `O(n)`. Space complexity: `O(1)`.

## Recursive approach

Shorter but uses `O(n)` stack space.

```go
func reverseRec(head *Node) *Node {
    if head == nil || head.Next == nil {
        return head
    }
    newHead := reverseRec(head.Next)
    head.Next.Next = head
    head.Next = nil
    return newHead
}
```

## Which one to use?

| Approach  | Time | Space | Notes                     |
|-----------|------|-------|---------------------------|
| Iterative | O(n) | O(1)  | Preferred for production  |
| Recursive | O(n) | O(n)  | Cleaner but deeper stack  |

In practice the iterative version wins. Stack overflow is a real
concern for long lists, and the iterative form is just as readable
once you have seen it a couple of times.
EOF

chunk=4
len=${#text}
i=0
while [ $i -lt $len ]; do
  emit "${text:$i:$chunk}"
  i=$((i + chunk))
done

printf 'data: [DONE]\n\n'
