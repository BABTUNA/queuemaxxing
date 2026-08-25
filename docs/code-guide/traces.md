# Detailed Core Execution Traces

[Back to Code Guide](../code-guide.md) · [Core Structures](core-concepts.md)

This guide follows real operations through the application one function at a time. Each step identifies the function, its source file and tier, its concrete input, and its output or state change.

Tiers describe source dependency depth, not runtime call order. Calls may move between higher and lower tiers.

## Trace 1: Create the `emails` Queue

### Starting state and request

```text
Manager.queues = {}
WAL file       = empty
```

```http
POST /queues
Content-Type: application/json

{"name":"emails","ordering":"fifo"}
```

### Complete call path

```text
Handler.ServeHTTP                         handler.go          Tier 5
└── Handler.createQueue                   handler.go          Tier 5
    ├── decodeJSON                        handler.go          Tier 5
    ├── Manager.CreateQueue               manager.go          Tier 4
    │   ├── storage.NewQueueJournal       storage/journal.go  Tier 3
    │   ├── queue.NewWithJournal          queue.go            Tier 2
    │   │   └── newQueue                  queue.go            Tier 2
    │   ├── storage.NewCreateQueueRecord  record.go           Tier 1
    │   └── WAL.Append                    wal.go              Tier 2
    │       ├── Record.validate           record.go           Tier 1
    │       └── writeAll                  wal.go              Tier 2
    └── writeJSON                         handler.go          Tier 5
```

### 1. `Handler.ServeHTTP` routes the request

File: [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go) — Tier 5

Input:

```text
method = POST
path   = /queues
body   = {"name":"emails","ordering":"fifo"}
```

The router matches `POST /queues` and calls `Handler.createQueue`. No queue data has changed yet.

### 2. `Handler.createQueue` calls `decodeJSON`

File: [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go) — Tier 5

`createQueue` starts with:

```go
var request createQueueRequest
```

Input to `decodeJSON`:

```text
destination → pointer to an empty createQueueRequest
request body → {"name":"emails","ordering":"fifo"}
```

Output written into the destination:

```go
createQueueRequest{
    Name:     "emails",
    Ordering: queue.FIFO,
}
```

`decodeJSON` also rejects unknown fields, oversized bodies, malformed JSON, and multiple JSON objects.

### 3. `Manager.CreateQueue` coordinates creation

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

Function call:

```go
h.manager.CreateQueue("emails", queue.FIFO)
```

Input:

```text
name     = "emails"
ordering = queue.FIFO
manager  = Manager{queues: {}, wal: sharedWAL}
```

The function first validates the name, then prepares the queue before acquiring the manager's exclusive map lock.

### 4. `storage.NewQueueJournal` binds the queue name to the WAL

File: [`internal/storage/journal.go`](../../internal/storage/journal.go) — Tier 3

Function call:

```go
storage.NewQueueJournal(m.wal, "emails")
```

Output:

```text
&storage.QueueJournal{
    wal:   pointer to sharedWAL,
    queue: "emails",
}
```

This journal ensures future enqueue and dequeue records are labeled as belonging to `"emails"`.

### 5. `queue.NewWithJournal` constructs the empty queue

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

Function call:

```go
queue.NewWithJournal(queue.FIFO, emailJournal)
```

After validating the ordering and non-nil journal, it calls `newQueue`.

Output:

```text
&queue.Queue{
    ordering:   FIFO,
    sequence:   0,
    ready:      readyHeap{items: [], ordering: FIFO},
    delayed:    delayedHeap[],
    generateID: generateMessageID,
    journal:    emailJournal,
}
```

The queue is constructed but has not yet been added to `Manager.queues`.

### 6. `storage.NewCreateQueueRecord` creates the durable operation

File: [`internal/storage/record.go`](../../internal/storage/record.go) — Tier 1

Function call:

```go
storage.NewCreateQueueRecord("emails", queue.FIFO)
```

Output:

```go
storage.Record{
    Operation: storage.CreateQueue,
    Queue:     "emails",
    Ordering:  queue.FIFO,
    Message:   nil,
    MessageID: "",
}
```

### 7. `WAL.Append` persists the creation record

File: [`internal/storage/wal.go`](../../internal/storage/wal.go) — Tier 2

Input: the `Record` from the previous step.

Internal calls:

1. `Record.validate` from [`record.go`](../../internal/storage/record.go) confirms a create record has a queue name and valid ordering.
2. `json.Marshal` converts the record to bytes.
3. A newline is appended.
4. The WAL mutex is acquired.
5. The file seeks to its end.
6. `writeAll` writes the complete line.
7. `file.Sync` makes the write durable before returning.

Bytes appended to the WAL:

```json
{"operation":"create_queue","queue":"emails","ordering":"fifo"}
```

Output:

```go
nil // write succeeded
```

### 8. `Manager.CreateQueue` commits the in-memory change

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

Only after `WAL.Append` succeeds:

```go
m.queues["emails"] = created
```

Manager state becomes:

```text
Manager{
    queues: {
        "emails" → pointer to the empty FIFO Queue
    },
    wal: sharedWAL,
}
```

Return value:

```go
service.QueueInfo{
    Name:     "emails",
    Ordering: queue.FIFO,
}, nil
```

### 9. `writeJSON` sends the HTTP response

File: [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go) — Tier 5

Input: status `201` and the returned `QueueInfo`.

Output:

```http
HTTP/1.1 201 Created
Content-Type: application/json

{"name":"emails","ordering":"fifo"}
```

## Trace 2: Enqueue an Immediately Available Message

### Starting state and request

Assume the current time is `2026-08-23T12:00:00Z` and ID generation returns `message-A`.

```text
emails Queue
├── sequence: 0
├── ready:   []
└── delayed: []
```

```http
POST /queues/emails/messages
Content-Type: application/json

{"body":"Password reset","priority":10,"delay_seconds":0}
```

### Complete call path

```text
Handler.enqueue                         handler.go          Tier 5
├── decodeJSON                          handler.go          Tier 5
├── Manager.Enqueue                     manager.go          Tier 4
│   ├── Manager.getQueue                manager.go          Tier 4
│   └── Queue.Enqueue                   queue.go            Tier 2
│       ├── validateEnqueueInput        queue.go            Tier 2
│       ├── generateMessageID           queue.go            Tier 2
│       ├── QueueJournal.RecordEnqueue  storage/journal.go  Tier 3
│       │   ├── NewEnqueueRecord        record.go           Tier 1
│       │   └── WAL.Append              wal.go              Tier 2
│       └── heap.Push                   standard library
│           └── readyHeap methods       ready_heap.go       Tier 1
└── writeJSON                           handler.go          Tier 5
```

### 1. `Handler.enqueue` decodes the request

File: [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go) — Tier 5

`decodeJSON` converts the body into:

```go
enqueueRequest{
    Body:         "Password reset",
    Priority:     10,
    DelaySeconds: 0,
}
```

The handler validates the delay and builds the queue-layer input:

```go
queue.EnqueueInput{
    Body:     "Password reset",
    Priority: 10,
    Delay:    0,
}
```

Function call:

```go
h.manager.Enqueue(
    "emails",
    enqueueInput,
    time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
)
```

### 2. `Manager.Enqueue` resolves the named queue

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

Input:

```text
name  = "emails"
input = EnqueueInput{Body: "Password reset", Priority: 10, Delay: 0}
now   = 2026-08-23T12:00:00Z
```

It calls `Manager.getQueue("emails")`. `getQueue` acquires a read lock and returns:

```text
pointer to emails Queue, nil
```

`Manager.Enqueue` then calls:

```go
q.Enqueue(input, now)
```

### 3. `Queue.Enqueue` creates the complete `Message`

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

`validateEnqueueInput` confirms the body and delay are valid. The queue mutex is then acquired so sequence assignment, journaling, and heap insertion happen as one protected operation.

`generateMessageID` returns `"message-A"`, and `Queue.Enqueue` builds:

```go
queue.Message{
    ID:          "message-A",
    Body:        "Password reset",
    Priority:    10,
    Sequence:    1,
    CreatedAt:   time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
    AvailableAt: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
}
```

At this point it is only a local value. The sequence field on the queue is still `0`, and neither heap contains the message.

### 4. `QueueJournal.RecordEnqueue` turns the message into a named operation

Implementation file: [`internal/storage/journal.go`](../../internal/storage/journal.go) — Tier 3
Interface definition: [`internal/queue/journal.go`](../../internal/queue/journal.go) — Tier 1

Function call made by `Queue.Enqueue`:

```go
q.journal.RecordEnqueue(message)
```

The runtime journal is:

```text
QueueJournal{
    queue: "emails",
    wal:   sharedWAL,
}
```

`QueueJournal.RecordEnqueue` calls:

```go
NewEnqueueRecord("emails", message)
```

### 5. `NewEnqueueRecord` produces the WAL data structure

File: [`internal/storage/record.go`](../../internal/storage/record.go) — Tier 1

Output:

```go
storage.Record{
    Operation: storage.Enqueue,
    Queue:     "emails",
    Ordering:  "",
    Message:   &message,
    MessageID: "",
}
```

### 6. `WAL.Append` writes and syncs the record

File: [`internal/storage/wal.go`](../../internal/storage/wal.go) — Tier 2

Input: the enqueue `Record`.

It validates, marshals, newline-terminates, locks, writes, and syncs the record. The resulting line contains the complete message:

```json
{"operation":"enqueue","queue":"emails","message":{"id":"message-A","body":"Password reset","priority":10,"sequence":1,"created_at":"2026-08-23T12:00:00Z","available_at":"2026-08-23T12:00:00Z"}}
```

Output:

```go
nil // the message is now durable
```

### 7. `Queue.Enqueue` commits the queue state

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

After journal success:

```go
q.sequence = 1
heap.Push(&q.ready, message)
```

`heap.Push` uses `readyHeap.Push`, `readyHeap.Less`, and possibly `readyHeap.Swap` from [`internal/queue/ready_heap.go`](../../internal/queue/ready_heap.go).

Queue state becomes:

```text
emails Queue
├── sequence: 1
├── ready:   [message-A]
└── delayed: []
```

`Queue.Enqueue` returns the complete `Message` and `nil`. `Manager.Enqueue` passes those values back unchanged.

### 8. `Handler.enqueue` writes the response

File: [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go) — Tier 5

Output:

```http
HTTP/1.1 201 Created
Content-Type: application/json

{"id":"message-A","body":"Password reset","priority":10,"sequence":1,"created_at":"2026-08-23T12:00:00Z","available_at":"2026-08-23T12:00:00Z"}
```

## Trace 3: Enqueue a Delayed Message

This trace shows exactly where delayed enqueue diverges from immediate enqueue.

### Starting state and request

```text
current time = 2026-08-23T12:00:00Z
emails.sequence = 1
```

```http
POST /queues/emails/messages

{"body":"Newsletter","priority":5,"delay_seconds":30}
```

### 1. `Handler.enqueue` creates an `EnqueueInput`

File: [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go) — Tier 5

Decoded HTTP structure:

```go
enqueueRequest{
    Body:         "Newsletter",
    Priority:     5,
    DelaySeconds: 30,
}
```

Queue-layer structure passed to `Manager.Enqueue`:

```go
queue.EnqueueInput{
    Body:     "Newsletter",
    Priority: 5,
    Delay:    30 * time.Second,
}
```

### 2. `Manager.Enqueue` and `Manager.getQueue` find `emails`

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

`getQueue("emails")` returns the existing queue pointer. `Manager.Enqueue` calls `Queue.Enqueue(input, now)`.

### 3. `Queue.Enqueue` creates a delayed `Message`

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

Assuming the generated ID is `message-B`, the local value becomes:

```go
queue.Message{
    ID:          "message-B",
    Body:        "Newsletter",
    Priority:    5,
    Sequence:    2,
    CreatedAt:   time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
    AvailableAt: time.Date(2026, 8, 23, 12, 0, 30, 0, time.UTC),
}
```

The `AvailableAt` difference comes from:

```go
createdAt.Add(input.Delay)
```

### 4. Journal and WAL persist the complete message

The calls and files are:

```text
QueueJournal.RecordEnqueue  internal/storage/journal.go  Tier 3
→ NewEnqueueRecord          internal/storage/record.go   Tier 1
→ WAL.Append                internal/storage/wal.go      Tier 2
```

Record input to `WAL.Append`:

```go
storage.Record{
    Operation: storage.Enqueue,
    Queue:     "emails",
    Message:   &messageB,
}
```

WAL output:

```json
{"operation":"enqueue","queue":"emails","message":{"id":"message-B","body":"Newsletter","priority":5,"sequence":2,"created_at":"2026-08-23T12:00:00Z","available_at":"2026-08-23T12:00:30Z"}}
```

### 5. `Queue.Enqueue` selects the delayed heap

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

Because `input.Delay != 0`, it calls:

```go
heap.Push(&q.delayed, message)
```

The standard library uses `delayedHeap.Push`, `delayedHeap.Less`, and possibly `delayedHeap.Swap` from [`internal/queue/delayed_heap.go`](../../internal/queue/delayed_heap.go) — Tier 1.

Final queue state:

```text
emails Queue
├── sequence: 2
├── ready:   [message-A]
└── delayed: [message-B available at 12:00:30]
```

The handler returns `message-B` with `201 Created`, but dequeue cannot return it before `12:00:30`.

## Trace 4: Dequeue and Promote a Delayed Message

### Starting state and request

Assume dequeue occurs at `2026-08-23T12:00:30Z`:

```text
ready heap root   = message-A(priority 1, sequence 1)
delayed heap root = message-B(priority 5, sequence 2, available 12:00:30)
```

```http
POST /queues/emails/dequeue
```

### Complete call path

```text
Handler.dequeue                         handler.go          Tier 5
└── Manager.Dequeue                     manager.go          Tier 4
    ├── Manager.getQueue                manager.go          Tier 4
    └── Queue.Dequeue                   queue.go            Tier 2
        ├── Queue.promoteEligible       queue.go            Tier 2
        │   ├── heap.Pop delayed        delayed_heap.go     Tier 1
        │   └── heap.Push ready         ready_heap.go       Tier 1
        ├── QueueJournal.RecordDequeue  storage/journal.go  Tier 3
        │   ├── NewDequeueRecord        record.go           Tier 1
        │   └── WAL.Append              wal.go              Tier 2
        └── heap.Pop ready              ready_heap.go       Tier 1
```

### 1. `Handler.dequeue` supplies the queue name and time

File: [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go) — Tier 5

Function call:

```go
h.manager.Dequeue(
    "emails",
    time.Date(2026, 8, 23, 12, 0, 30, 0, time.UTC),
)
```

### 2. `Manager.Dequeue` resolves the queue

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

`Manager.getQueue("emails")` returns the queue pointer, and `Manager.Dequeue` calls:

```go
q.Dequeue(now)
```

### 3. `Queue.Dequeue` locks and calls `promoteEligible`

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

Input state:

```text
now     = 12:00:30
ready   = [A priority 1]
delayed = [B priority 5, available 12:00:30]
```

`promoteEligible` evaluates:

```go
!q.delayed[0].AvailableAt.After(now)
```

For B, `12:00:30.After(12:00:30)` is false, so the negation is true: B is eligible.

### 4. `heap.Pop(&q.delayed)` removes the earliest eligible message

Heap implementation: [`internal/queue/delayed_heap.go`](../../internal/queue/delayed_heap.go) — Tier 1

The standard library reorganizes the delayed heap using its `Len`, `Less`, and `Swap` methods, then invokes its `Pop` method.

Output:

```text
message-B
```

Intermediate state:

```text
delayed = []
ready   = [message-A]
```

### 5. `heap.Push(&q.ready, messageB)` ranks the promoted message

Heap implementation: [`internal/queue/ready_heap.go`](../../internal/queue/ready_heap.go) — Tier 1

Input:

```text
existing root = A(priority 1)
new message   = B(priority 5)
```

`readyHeap.Less` determines that B ranks before A because `5 > 1`.

Resulting logical dequeue order:

```text
B(priority 5), A(priority 1)
```

Only the heap root is guaranteed to be the next item; the underlying slice is not a completely sorted list.

### 6. `Queue.Dequeue` selects but does not remove B yet

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

The selected value is:

```go
message := q.ready.items[0] // message-B
```

The message remains in the heap until its dequeue is durable.

### 7. `QueueJournal.RecordDequeue` creates the removal record

File: [`internal/storage/journal.go`](../../internal/storage/journal.go) — Tier 3

Function call:

```go
q.journal.RecordDequeue("message-B")
```

The concrete journal adds the queue name and calls `NewDequeueRecord` from [`internal/storage/record.go`](../../internal/storage/record.go) — Tier 1.

Record output:

```go
storage.Record{
    Operation: storage.Dequeue,
    Queue:     "emails",
    MessageID: "message-B",
}
```

### 8. `WAL.Append` makes the removal durable

File: [`internal/storage/wal.go`](../../internal/storage/wal.go) — Tier 2

WAL line:

```json
{"operation":"dequeue","queue":"emails","message_id":"message-B"}
```

Once `file.Sync` succeeds, `WAL.Append` returns `nil`.

### 9. `heap.Pop(&q.ready)` removes B from memory

Heap implementation: [`internal/queue/ready_heap.go`](../../internal/queue/ready_heap.go) — Tier 1

Output from `Queue.Dequeue`:

```text
Message{ID: "message-B", Body: "Newsletter", Priority: 5, ...}, true, nil
```

Final queue state:

```text
ready   = [message-A]
delayed = []
```

`Manager.Dequeue` passes the values back unchanged. `Handler.dequeue` calls `writeJSON` and responds:

```http
HTTP/1.1 200 OK

{"id":"message-B","body":"Newsletter","priority":5,"sequence":2,...}
```

If the ready heap were empty after promotion, `Queue.Dequeue` would instead return `Message{}, false, nil`, and the handler would respond with `204 No Content`.

## Trace 5: WAL Failure During Enqueue

This trace shows why the journal call occurs before the queue changes memory.

### Starting state

```text
emails Queue
├── sequence: 3
├── ready:   []
└── journal → QueueJournal → closed WAL
```

### 1. The normal HTTP and manager path succeeds

Files: [`handler.go`](../../internal/httpapi/handler.go) — Tier 5; [`manager.go`](../../internal/service/manager.go) — Tier 4

The request becomes:

```go
queue.EnqueueInput{
    Body:     "Generate report",
    Priority: 2,
    Delay:    0,
}
```

`Manager.getQueue("emails")` finds the queue and calls `Queue.Enqueue`.

### 2. `Queue.Enqueue` constructs a candidate message

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

Local value:

```go
queue.Message{
    ID:       "message-C",
    Body:     "Generate report",
    Priority: 2,
    Sequence: 4,
    // timestamps omitted here
}
```

Important state at this moment:

```text
local message.Sequence = 4
q.sequence             = 3
q.ready                = []
```

The candidate exists only inside the function; it has not been committed.

### 3. The journal call reaches the closed WAL

Call path:

```text
Queue.Enqueue
→ QueueJournal.RecordEnqueue    internal/storage/journal.go
→ NewEnqueueRecord              internal/storage/record.go
→ WAL.Append                    internal/storage/wal.go
```

Input to `WAL.Append` is a valid enqueue `Record`, but `WAL.Append` observes:

```go
w.closed == true
```

Output:

```go
storage.ErrClosed
```

### 4. Each caller propagates the failure

```text
WAL.Append
returns ErrClosed

QueueJournal.RecordEnqueue
returns ErrClosed

Queue.Enqueue
returns Message{}, "record enqueue: WAL is closed"

Manager.Enqueue
returns the same zero Message and wrapped error

Handler.enqueue
calls writeServiceError
```

[`writeServiceError`](../../internal/httpapi/handler.go) does not expose internal storage details and returns:

```http
HTTP/1.1 500 Internal Server Error

{"error":{"code":"internal_error","message":"internal server error"}}
```

### 5. State remains unchanged

Because execution returned before sequence assignment and `heap.Push`:

```text
q.sequence = 3
q.ready    = []
WAL        = contains no record for message-C
```

The failed message exists in neither durable state nor live state.

## Trace 6: Restart and Recover All Queue State

### Starting WAL

Assume the process restarts at `2026-08-23T12:10:00Z` with this file:

```json
{"operation":"create_queue","queue":"emails","ordering":"fifo"}
{"operation":"enqueue","queue":"emails","message":{"id":"A","body":"First","priority":1,"sequence":1,"created_at":"2026-08-23T12:00:00Z","available_at":"2026-08-23T12:00:00Z"}}
{"operation":"enqueue","queue":"emails","message":{"id":"B","body":"Later","priority":5,"sequence":2,"created_at":"2026-08-23T12:00:00Z","available_at":"2026-08-23T12:15:00Z"}}
{"operation":"dequeue","queue":"emails","message_id":"A"}
```

Expected live result:

```text
emails queue exists
sequence = 2
ready    = []
delayed  = [B]
```

### Complete call path

```text
main
└── run                              cmd/server/main.go     Tier 6
    ├── storage.Open                 storage/wal.go         Tier 2
    ├── service.NewManager           service/manager.go     Tier 4
    │   ├── WAL.Replay               storage/wal.go         Tier 2
    │   │   └── Record.validate      storage/record.go      Tier 1
    │   ├── storage.Recover          storage/recovery.go    Tier 2
    │   ├── storage.NewQueueJournal  storage/journal.go     Tier 3
    │   └── queue.Restore            queue/queue.go         Tier 2
    │       ├── queue.NewWithJournal queue/queue.go         Tier 2
    │       └── heap.Push delayed    delayed_heap.go        Tier 1
    └── httpapi.NewHandler           httpapi/handler.go     Tier 5
```

### 1. `run` calls `storage.Open`

Entry file: [`cmd/server/main.go`](../../cmd/server/main.go) — Tier 6
Function file: [`internal/storage/wal.go`](../../internal/storage/wal.go) — Tier 2

Function call:

```go
storage.Open("./data/queue.wal")
```

`Open` creates the parent directory if necessary and opens the existing file for reading and writing.

Output:

```text
&storage.WAL{
    file:   open handle for ./data/queue.wal,
    closed: false,
}
```

### 2. `run` calls `service.NewManager`

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

Input:

```text
wal = open WAL from step 1
now = 2026-08-23T12:10:00Z
```

The manager cannot construct live queues yet; it first needs logical state from the WAL.

### 3. `WAL.Replay` reads physical records

File: [`internal/storage/wal.go`](../../internal/storage/wal.go) — Tier 2

`Replay` locks the WAL, seeks to the beginning, and reads one newline-terminated byte slice at a time. For each line it calls `json.Unmarshal` and `Record.validate`.

Output:

```go
[]storage.Record{
    {Operation: CreateQueue, Queue: "emails", Ordering: queue.FIFO},
    {Operation: Enqueue, Queue: "emails", Message: &messageA},
    {Operation: Enqueue, Queue: "emails", Message: &messageB},
    {Operation: Dequeue, Queue: "emails", MessageID: "A"},
}
```

The slice preserves WAL order. `Replay` does not create queues or decide which messages remain.

If the final file bytes do not end in a newline, `Replay` treats them as an interrupted append and truncates that partial line.

### 4. `storage.Recover` builds logical state

File: [`internal/storage/recovery.go`](../../internal/storage/recovery.go) — Tier 2

Input: the four-record slice from `Replay`.

It begins with:

```text
queues = {}
```

After the create record:

```text
queues = {
    "emails" → recoveredQueue{
        ordering: FIFO,
        sequence: 0,
        messages: {},
    }
}
```

After enqueue A:

```text
emails.sequence = 1
emails.messages = {"A": messageA}
```

After enqueue B:

```text
emails.sequence = 2
emails.messages = {
    "A": messageA,
    "B": messageB,
}
```

After dequeue A:

```text
emails.sequence = 2
emails.messages = {"B": messageB}
```

The highest observed sequence remains `2` even though messages can be deleted.

`Recover` then converts its internal map into the public recovery structure:

```go
map[string]queue.State{
    "emails": {
        Ordering: queue.FIFO,
        Sequence: 2,
        Messages: []queue.Message{messageB},
    },
}
```

That map is returned to `NewManager`.

### 5. `NewManager` allocates the registry

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

Intermediate manager:

```text
Manager{
    queues: empty map with capacity 1,
    wal:    open shared WAL,
}
```

For the `"emails"` state, it next reconstructs the queue's journal and runtime structures.

### 6. `storage.NewQueueJournal` restores the persistence connection

File: [`internal/storage/journal.go`](../../internal/storage/journal.go) — Tier 3

Input:

```text
wal  = shared open WAL
name = "emails"
```

Output:

```text
QueueJournal{
    queue: "emails",
    wal:   sharedWAL,
}
```

This is a new in-memory adapter. It is not itself read from the WAL.

### 7. `queue.Restore` rebuilds the live queue

File: [`internal/queue/queue.go`](../../internal/queue/queue.go) — Tier 2

Function call:

```go
queue.Restore(emailState, restartTime, emailJournal)
```

Input:

```go
queue.State{
    Ordering: queue.FIFO,
    Sequence: 2,
    Messages: []queue.Message{messageB},
}
```

`Restore` calls `NewWithJournal`, assigns:

```go
q.sequence = state.Sequence // 2
```

It then checks each surviving message against `now`:

```text
B.AvailableAt = 12:15
restart now   = 12:10
12:15 is after 12:10 → delayed heap
```

It calls `heap.Push(&q.delayed, messageB)`, using the implementation in [`internal/queue/delayed_heap.go`](../../internal/queue/delayed_heap.go) — Tier 1.

Output:

```text
&queue.Queue{
    ordering: FIFO,
    sequence: 2,
    ready:    [],
    delayed:  [messageB],
    journal:  emailJournal,
}
```

Historical messages are not journaled again during restoration.

### 8. `NewManager` registers the restored queue

File: [`internal/service/manager.go`](../../internal/service/manager.go) — Tier 4

```go
manager.queues["emails"] = restoredQueue
```

Final output from `NewManager`:

```text
Manager{
    queues: {
        "emails" → restored FIFO Queue
    },
    wal: sharedWAL,
}
```

`run` passes the manager to `httpapi.NewHandler` in [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go), then starts the HTTP server.

Because the restored sequence is `2`, the next accepted enqueue receives sequence `3`. Message B stays delayed until `12:15` and then becomes eligible during a dequeue call.
