# `internal/queue/message.go`

[Back to Code Guide](../code-guide.md) · [Source File](../../internal/queue/message.go)

## Why This File Exists

This file defines the core data vocabulary of the entire project. It does not perform queue operations. Instead, it defines what ordering, messages, enqueue requests, and recovered queue state look like.

Most higher-level files cannot operate until these types exist.

## Dependencies

Project dependencies: None.

Standard-library dependency:

```go
import "time"
```

The file uses `time.Duration` for requested delays and `time.Time` for message timestamps.

## Files That Depend on It

| File | What it uses |
| --- | --- |
| `ready_heap.go` ([source](../../internal/queue/ready_heap.go)) | `Message` and `Ordering` |
| `delayed_heap.go` ([source](../../internal/queue/delayed_heap.go)) | `Message` |
| `queue/journal.go` ([source](../../internal/queue/journal.go)) | `Message` |
| `queue.go` ([source](../../internal/queue/queue.go)) | Every type in this file |
| `storage/record.go` ([source](../../internal/storage/record.go)) | `Ordering` and `Message` |
| `storage/recovery.go` ([source](../../internal/storage/recovery.go)) | `Message` and `State` |
| `service/manager.go` ([source](../../internal/service/manager.go)) | `Ordering`, `EnqueueInput`, and `Message` |
| `httpapi/handler.go` ([source](../../internal/httpapi/handler.go)) | `Ordering`, `EnqueueInput`, and `Message` responses |
| `client/client.go` ([source](../../internal/client/client.go)) | `Ordering` and `Message` responses |

## Limits

```go
const MaxBodyBytes = 256 * 1024
const MaxDelay = 15 * time.Minute
```

These constants define the queue contract:

- A body can contain at most 256 KiB of UTF-8 data.
- A message can be delayed for at most 15 minutes, or 900 seconds.

They are used later by `queue.go`, `handler.go`, and `cmd/client/main.go` when validating input.

## `Ordering`

```go
type Ordering string

const (
    FIFO Ordering = "fifo"
    LIFO Ordering = "lifo"
)
```

Purpose: Describe how equal-priority messages are ordered.

Possible values:

```go
queue.FIFO
queue.LIFO
```

Example:

```go
ordering := queue.FIFO
```

The underlying value is a string:

```text
"fifo"
```

This allows it to appear naturally in JSON:

```json
{"name":"emails","ordering":"fifo"}
```

How it moves through the system:

```text
HTTP create request
→ createQueueRequest.Ordering
→ Manager.CreateQueue
→ Queue.ordering
→ readyHeap.ordering
→ readyHeap.Less chooses FIFO or LIFO behavior
```

## `Message`

```go
type Message struct {
    ID          string
    Body        string
    Priority    int32
    Sequence    uint64
    CreatedAt   time.Time
    AvailableAt time.Time
}
```

Purpose: Represent a complete message accepted by the server.

Example value:

```go
queue.Message{
    ID:          "550e8400-e29b-41d4-a716-446655440000",
    Body:        "Send newsletter",
    Priority:    5,
    Sequence:    12,
    CreatedAt:   time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
    AvailableAt: time.Date(2026, 8, 23, 12, 0, 30, 0, time.UTC),
}
```

Field meanings:

| Field | Assigned by | Meaning |
| --- | --- | --- |
| `ID` | Queue | Unique server-generated identifier |
| `Body` | Client | Message content |
| `Priority` | Client | Higher values dequeue first |
| `Sequence` | Queue | Increasing number scoped to one queue |
| `CreatedAt` | Queue | UTC enqueue time |
| `AvailableAt` | Queue | Earliest eligible dequeue time |

JSON output:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "body": "Send newsletter",
  "priority": 5,
  "sequence": 12,
  "created_at": "2026-08-23T12:00:00Z",
  "available_at": "2026-08-23T12:00:30Z"
}
```

The text after each struct field, such as:

```go
ID string `json:"id"`
```

is a JSON struct tag. It tells Go's JSON encoder to use `"id"` rather than `"ID"`.

### Message Walkthrough

Before enqueue, the client does not control the ID, sequence, or timestamps:

```go
EnqueueInput{
    Body:     "Send newsletter",
    Priority: 5,
    Delay:    30 * time.Second,
}
```

`Queue.Enqueue` adds server-controlled metadata:

```text
ID          ← generated
Sequence    ← previous queue sequence + 1
CreatedAt   ← current server time
AvailableAt ← CreatedAt + Delay
```

Result:

```go
Message{
    ID:          "generated-id",
    Body:        "Send newsletter",
    Priority:    5,
    Sequence:    12,
    CreatedAt:   12:00:00,
    AvailableAt: 12:00:30,
}
```

The same value then travels through:

```text
Queue.Enqueue
→ QueueJournal.RecordEnqueue
→ WAL enqueue record
→ ready or delayed heap
→ HTTP response
```

## `EnqueueInput`

```go
type EnqueueInput struct {
    Body     string
    Priority int32
    Delay    time.Duration
}
```

Purpose: Hold only the fields needed to request a new message.

Input example:

```go
queue.EnqueueInput{
    Body:     "Send newsletter",
    Priority: 5,
    Delay:    30 * time.Second,
}
```

Output: This type does not produce an output by itself. `Queue.Enqueue` accepts it and returns a complete `Message`.

Example function call:

```go
message, err := q.Enqueue(
    queue.EnqueueInput{
        Body:     "Send newsletter",
        Priority: 5,
        Delay:    30 * time.Second,
    },
    time.Now(),
)
```

Why it is separate from `Message`:

```text
EnqueueInput = client-controlled fields
Message      = accepted message plus server-controlled metadata
```

HTTP conversion example:

```json
{"body":"Send newsletter","priority":5,"delay_seconds":30}
```

becomes:

```go
EnqueueInput{
    Body:     "Send newsletter",
    Priority: 5,
    Delay:    30 * time.Second,
}
```

## `State`

```go
type State struct {
    Ordering Ordering
    Sequence uint64
    Messages []Message
}
```

Purpose: Carry the durable information needed to restore one queue after restart.

Example:

```go
queue.State{
    Ordering: queue.FIFO,
    Sequence: 12,
    Messages: []queue.Message{
        {
            ID:       "message-B",
            Body:     "Still waiting",
            Sequence: 12,
        },
    },
}
```

The `Sequence` may be higher than the number of remaining messages because dequeued messages are no longer included.

Example:

```text
Sequences ever created: 1, 2, 3
Messages 1 and 2 dequeued

State.Sequence = 3
State.Messages = [message with sequence 3]
```

### State Recovery Walkthrough

WAL history:

```text
Create emails FIFO
Enqueue A sequence 1
Enqueue B sequence 2
Dequeue A
```

`storage.Recover` produces:

```go
State{
    Ordering: queue.FIFO,
    Sequence: 2,
    Messages: []Message{messageB},
}
```

`queue.Restore` consumes that state:

```go
restored, err := queue.Restore(state, time.Now(), journal)
```

It restores:

```text
ordering = FIFO
sequence = 2
message B → ready or delayed heap based on AvailableAt
```

The next enqueue receives sequence `3`, not `1`.

## What to Remember

If asked about this file, the short answer is:

> `message.go` defines the shared queue data model. `EnqueueInput` contains client-controlled fields, `Message` adds server-controlled metadata, `State` carries durable recovery data, and `Ordering` controls FIFO or LIFO tie-breaking. It is a Tier 0 file because the rest of the project builds on these definitions.
