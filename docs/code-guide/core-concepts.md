# Core Project Structures

[Back to Code Guide](../code-guide.md) · [Core Traces](traces.md)

This guide covers only the structures Queuemaxxing defines to represent messages, order work, persist mutations, and rebuild queues. General Go concepts are intentionally excluded.

## 1. Message Model

Source: [`internal/queue/message.go`](../../internal/queue/message.go) — Tier 0

Three related types represent a message at different stages.

### `EnqueueInput`: client-controlled fields

```go
EnqueueInput{
    Body:     "send receipt",
    Priority: 10,
    Delay:    30 * time.Second,
}
```

The client chooses these values. It cannot choose the identity, sequence, or timestamps.

### `Message`: accepted queue entry

[`Queue.Enqueue`](../../internal/queue/queue.go) converts the input into:

```go
Message{
    ID:          "8a40...",
    Body:        "send receipt",
    Priority:    10,
    Sequence:    7,
    CreatedAt:   12:00:00,
    AvailableAt: 12:00:30,
}
```

The fields serve distinct ordering and recovery purposes:

| Field | Role |
| --- | --- |
| `ID` | Stable identity used by dequeue WAL records |
| `Priority` | Higher values rank ahead of lower values |
| `Sequence` | Per-queue enqueue order and deterministic tie-breaker |
| `AvailableAt` | Determines whether the message belongs in the delayed or ready heap |

### `Ordering`: equal-priority policy

`FIFO` selects the lower sequence first; `LIFO` selects the higher sequence first. Priority is always compared before ordering mode.

```text
A(priority 5, sequence 1)
B(priority 5, sequence 2)

FIFO → A, B
LIFO → B, A
```

## 2. Live `Queue`

Source: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

`Queue` is the core in-memory aggregate:

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

Example runtime state:

```text
Queue
├── ordering: FIFO
├── sequence: 7
├── ready:   [A(priority 10), B(priority 2)]
├── delayed: [C(available at 12:05)]
└── journal: QueueJournal for "emails"
```

Its mutex protects both heaps and sequence assignment as one unit. An enqueue therefore cannot obtain a sequence without also completing the corresponding heap and journal operation under the same lock.

The queue intentionally has no name. Naming belongs to the manager; persistence receives the name through the queue-specific journal.

## 3. Ready and Delayed Heaps

Sources: [`ready_heap.go`](../../internal/queue/ready_heap.go) and [`delayed_heap.go`](../../internal/queue/delayed_heap.go) — Tier 1

The two heaps represent different questions:

| Structure | Contains | Top element |
| --- | --- | --- |
| `readyHeap` | Messages eligible now | Next message to consume |
| `delayedHeap` | Messages ineligible until later | Next message to become eligible |

`readyHeap.Less` ranks by higher priority, then applies FIFO/LIFO to `Sequence`:

```text
A(priority 4, sequence 1)
B(priority 9, sequence 2)
C(priority 4, sequence 3)

FIFO dequeue order → B, A, C
LIFO dequeue order → B, C, A
```

`delayedHeap.Less` ranks by the earliest `AvailableAt`, using the lower sequence only when timestamps match:

```text
C available at 12:05, sequence 3
D available at 12:03, sequence 4

delayed heap top → D
```

[`Queue.promoteEligible`](../../internal/queue/queue.go) repeatedly moves the delayed heap's top message into the ready heap once `AvailableAt <= now`. Priority affects consumption only after promotion.

## 4. Journal Boundary

Sources: [`queue/journal.go`](../../internal/queue/journal.go) — Tier 1; [`storage/journal.go`](../../internal/storage/journal.go) — Tier 3

`Journal` is the queue engine's persistence boundary:

```go
type Journal interface {
    RecordEnqueue(Message) error
    RecordDequeue(messageID string) error
}
```

The important project-specific relationship is:

```text
Queue
└── Journal interface
    └── QueueJournal
        ├── queue name: "emails"
        └── shared WAL
```

The engine supplies the message operation; `QueueJournal` adds the queue name and converts it into a WAL record. This keeps queue ordering independent from disk format while ensuring persistence happens before memory changes.

## 5. WAL `Record`

Source: [`internal/storage/record.go`](../../internal/storage/record.go) — Tier 1

A `Record` is one durable state transition:

```go
type Record struct {
    Operation Operation
    Queue     string
    Ordering  queue.Ordering
    Message   *queue.Message
    MessageID string
}
```

Only the fields relevant to the operation are populated:

```json
{"operation":"create_queue","queue":"emails","ordering":"fifo"}
{"operation":"enqueue","queue":"emails","message":{"id":"A","sequence":1}}
{"operation":"dequeue","queue":"emails","message_id":"A"}
```

An enqueue stores the complete message because recovery must reconstruct it. A dequeue needs only the message ID because recovery removes that ID from the reconstructed message set.

[`WAL.Append`](../../internal/storage/wal.go) writes each record as one JSON line and syncs it before the queue mutates memory.

## 6. Recovery `State`

Sources: [`internal/queue/message.go`](../../internal/queue/message.go) — Tier 0; [`storage/recovery.go`](../../internal/storage/recovery.go) — Tier 2

`State` is the transfer format between WAL recovery and a live queue:

```go
State{
    Ordering: FIFO,
    Sequence: 7,
    Messages: []Message{messageB, messageC},
}
```

Recovery applies records in order:

```text
create emails
enqueue A(sequence 6)
enqueue B(sequence 7)
dequeue A

State → ordering FIFO, sequence 7, messages [B]
```

The stored sequence remains `7`, even though message A was removed. This prevents the next enqueue from reusing an earlier sequence. [`queue.Restore`](../../internal/queue/queue.go) then divides surviving messages between the ready and delayed heaps using the current time.

## 7. `Manager` Queue Registry

Source: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

`Manager` connects external queue names to live queue instances and one shared WAL:

```go
type Manager struct {
    mu     sync.RWMutex
    queues map[string]*queue.Queue
    wal    *storage.WAL
}
```

Example:

```text
Manager
├── queues["emails"] → FIFO Queue + journal("emails")
├── queues["jobs"]   → LIFO Queue + journal("jobs")
└── wal               → both journals append here
```

The manager lock protects the registry itself. Each `Queue` has a separate lock for its messages, so operations on different queues do not share the queue-level critical section.

## How the Structures Connect

```text
EnqueueInput
→ Queue creates Message
→ QueueJournal creates Record
→ WAL persists Record
→ Message enters readyHeap or delayedHeap

On restart:
WAL Records
→ recovery State
→ Queue with rebuilt heaps
→ Manager registry
```

For function-by-function execution, continue with the [core traces](traces.md).
