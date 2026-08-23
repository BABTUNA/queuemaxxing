# Core Queue Engine

[Repository](../README.md) · [Queue Contract](queue-contract.md) · **Core Queue Engine** · [Durable Storage](durable-storage.md) · [HTTP Service](http-service.md) · [Demo Client](demo-client.md)

The core engine implements priority, FIFO/LIFO tie-breaking, delay, and thread-safe enqueue/dequeue behavior. It is intentionally independent of HTTP and persistence.

## Source Navigation

| File | Responsibility |
| --- | --- |
| [`message.go`](../internal/queue/message.go) | Ordering constants, message model, input model, and limits |
| [`errors.go`](../internal/queue/errors.go) | Validation and sequence errors |
| [`ready_heap.go`](../internal/queue/ready_heap.go) | Priority plus FIFO/LIFO ordering |
| [`delayed_heap.go`](../internal/queue/delayed_heap.go) | Earliest-availability ordering |
| [`journal.go`](../internal/queue/journal.go) | Persistence boundary used before memory mutations |
| [`queue.go`](../internal/queue/queue.go) | Queue construction, restoration, enqueue, dequeue, promotion, and IDs |
| [`queue_test.go`](../internal/queue/queue_test.go) | Contract, boundary, and concurrency tests |
| [`persistence_test.go`](../internal/queue/persistence_test.go) | Verifies failed journal writes do not mutate the queue |

## Queue State

```go
type Queue struct {
    mu         sync.Mutex
    ordering   Ordering
    sequence   uint64
    ready      readyHeap
    delayed    delayedHeap
    generateID idGenerator
    journal    Journal
}
```

- `mu` makes each operation atomic.
- `sequence` supplies deterministic FIFO/LIFO tie-breaking.
- `ready` contains immediately consumable messages.
- `delayed` contains messages waiting for `AvailableAt`.
- `generateID` uses UUIDv4 generation and can be replaced in tests.
- `journal` records enqueue and dequeue before they mutate memory.

## Ready Heap

The ready heap ranks messages by:

1. Higher priority.
2. Lower sequence for FIFO or higher sequence for LIFO.

```go
if left.Priority != right.Priority {
    return left.Priority > right.Priority
}
if h.ordering == FIFO {
    return left.Sequence < right.Sequence
}
return left.Sequence > right.Sequence
```

Go's `container/heap` package controls heap operations and calls the custom `Len`, `Less`, `Swap`, `Push`, and `Pop` methods.

## Delayed Heap

The delayed heap puts the earliest `AvailableAt` at the root. Equal timestamps use the lower sequence as a deterministic tie-breaker.

```go
if !h[i].AvailableAt.Equal(h[j].AvailableAt) {
    return h[i].AvailableAt.Before(h[j].AvailableAt)
}
return h[i].Sequence < h[j].Sequence
```

The delayed heap only decides which message becomes eligible next. Once promoted, the ready heap decides dequeue order.

## Enqueue Flow

```text
validate input
→ lock queue
→ check sequence capacity
→ generate ID
→ assign sequence and timestamps
→ push to ready or delayed heap
→ unlock
```

Immediate messages enter `ready`; positive-delay messages enter `delayed`. Validation or ID-generation failures do not mutate queue state.

## Dequeue Flow

```text
lock queue
→ promote every delayed message where AvailableAt <= now
→ pop the highest-ranked ready message
→ unlock
```

If the ready heap is empty, dequeue returns `(Message{}, false, nil)`. Persistence failures are returned as the third value without removing the message.

## Concurrency

One mutex protects sequence assignment and both heaps. This makes operations on a queue serializable and prevents two consumers from dequeuing the same message. Separate queue instances do not share a lock.

## Verification

The test suite covers:

- FIFO, LIFO, priority, and negative priorities
- Delay and the exact availability boundary
- Sequence preservation after delayed promotion
- Input limits, ID errors, and sequence exhaustion
- Concurrent enqueue, dequeue, and mixed operations

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
```
