#!/usr/bin/env bash
# Emit a long-form response that overflows the viewport, used to demo
# the alt-screen mode's scrolling.

set -e

emit() {
  local content="$1"
  printf 'data: {"choices":[{"delta":{"content":%s}}],"model":"demo"}\n\n' \
    "$(printf '%s' "$content" | jq -Rs .)"
  sleep 0.02
}

text='# Reversing a Linked List in Go

There are two classic approaches: **iterative** and **recursive**.

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

The recursive version is shorter but uses `O(n)` stack space.

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

| Approach  | Time | Space | Notes |
|-----------|------|-------|-------|
| Iterative | O(n) | O(1)  | Preferred for production code |
| Recursive | O(n) | O(n)  | Cleaner but deeper call stack |

In practice the iterative version wins. Stack overflow is a real
concern for long lists, and the iterative form is just as readable
once you have seen it a couple of times.'

while IFS= read -r line; do
  for word in $line; do
    emit "$word "
  done
  emit $'\n'
done <<< "$text"

printf 'data: [DONE]\n\n'
