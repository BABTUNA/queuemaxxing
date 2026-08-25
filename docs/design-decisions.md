# Design Decisions and Future Work

[Repository](../README.md) · [Queue Contract](queue-contract.md) · [Durable Storage](durable-storage.md) · [Validation](validation.md)

This document answers the design questions included with the take-home. It distinguishes the behavior implemented today from changes that would require a new delivery contract.

## Current Design in One Paragraph

Queuemaxxing is a single-process, single-machine HTTP queue. Each named queue combines priority with FIFO or LIFO tie-breaking and supports delayed eligibility. Successful create, enqueue, and destructive dequeue operations are synced to an append-only WAL before memory changes. The current dequeue contract is deliberately at-most-once: there is no acknowledgement phase, visibility timeout, or automatic retry.

## How Do You Handle Replay Messages?

I interpret replay as rebuilding queue state after an application restart.

Every successful queue mutation is written to the WAL before memory changes. During startup, the application reads those records in order:

```text
create_queue → recreate the queue
enqueue      → restore the message
dequeue      → remove the message
```

After replaying all records, the remaining messages are placed into either the ready or delayed heap based on their `AvailableAt` timestamp. The highest recovered sequence is also restored so sequence numbers are never reused.

An incomplete final WAL record is treated as an interrupted write and removed. A complete but invalid record is treated as corruption and stops startup rather than being silently skipped.

> **Interpretation note:** I interpret “replay messages” primarily as reconstructing queue state after an application restart because the requirements emphasize durability and restart protection. If replay instead means redelivering a message after consumer failure, the current version does not support it because dequeue is destructive and at-most-once. Supporting that behavior would require acknowledgements, visibility timeouts, and durable in-flight message state.

## How Would You Refactor the Queue into Pub/Sub?

The simplest refactor would reuse each existing `Queue` as one Pub/Sub subscription.

Today, the manager maps one external name to one queue:

```text
Manager
└── queues
    └── "emails" → Queue
        ├── ready heap
        ├── delayed heap
        ├── FIFO/LIFO ordering
        └── journal
```

An enqueue follows the existing path:

```text
Manager.Enqueue
→ Queue.Enqueue
→ Journal
→ WAL
→ ready or delayed heap
```

For Pub/Sub, I would add a topic and fan-out layer above the current queue manager. Every subscription would be backed by an existing queue:

```text
Topic: user-created
├── Subscription: email-service
│   └── existing Queue
├── Subscription: analytics
│   └── existing Queue
└── Subscription: audit-log
    └── existing Queue
```

Each subscription would therefore reuse the current:

- `Queue.Enqueue` and `Queue.Dequeue`
- ready and delayed heaps
- priority and FIFO/LIFO ordering
- queue mutex
- journal and WAL durability
- restart-recovery pattern

Suppose a publisher sends this logical event A:

```json
{"event":"user_created","user_id":"123"}
```

The new `PubSubManager.Publish` function would fan it out to each subscription queue:

```text
PubSubManager.Publish("user-created", event A)

→ email-service.Queue.Enqueue(A)
→ analytics.Queue.Enqueue(A)
→ audit-log.Queue.Enqueue(A)
```

Each call creates a separate delivery copy using the existing queue engine. The copies can share a logical event identifier while retaining their own queue message IDs and sequences:

```text
email-service Queue.ready → [A-email]
analytics Queue.ready     → [A-analytics]
audit-log Queue.ready     → [A-audit]
```

When the email service consumes its copy:

```text
email-service.Queue.Dequeue()
```

only that subscription queue changes:

```text
email-service Queue.ready → []
analytics Queue.ready     → [A-analytics]
audit-log Queue.ready     → [A-audit]
```

The persistence layer would add topic and subscription metadata records. A publish should be represented as one durable fan-out operation so a crash cannot deliver the event to only some subscriptions. WAL recovery would rebuild that fan-out into each subscription's existing queue state. Per-subscription dequeues would then be recorded independently:

```text
publish user-created A
dequeue user-created/email-service A-email
```

After restart, recovery would reconstruct:

```text
email-service → []
analytics     → [A-analytics]
audit-log     → [A-audit]
```

The genuinely new code would be the topic/subscription registry, publish fan-out, and its WAL record types. The existing queue remains the delivery engine for each subscription. Storing one physical message with per-subscription references could be added later as a storage optimization, but copying into existing queues is the clearest minimal refactor.

## What Would You Add with More Time?

I would focus on three additions that directly address the current queue's largest delivery and storage limitations.

### 1. ACK/NACK with visibility timeouts

Currently, dequeue permanently removes a message immediately:

```text
ready: [A]

Worker receives A
→ ready: []
```

If the worker crashes before processing A, the message is lost. With ACK/NACK, receiving A would move it into an in-flight collection instead:

```text
Before receive:
ready:     [A]
in-flight: []

After receive:
ready:     []
in-flight: [A]
```

The server would return the message with an opaque receipt handle:

```json
{
  "message": {"id":"A","body":"Send email"},
  "receipt_handle":"receipt-123"
}
```

If processing succeeds, the worker sends ACK and the queue permanently deletes A:

```text
POST /messages/receipt-123/ack

ready:     []
in-flight: []
```

If processing fails, the worker sends NACK and A returns to the ready heap:

```text
POST /messages/receipt-123/nack

ready:     [A]
in-flight: []
```

If the worker crashes and sends neither response, A returns to the ready heap when its visibility timeout expires. Reserve, ACK, NACK, and timeout transitions would be written to the WAL so in-flight state survives application restarts.

### 2. Retries and dead-letter queues

Retries build on the ACK/NACK workflow. Each message would track its delivery attempts and configured limit:

```text
Message A
delivery_attempts: 0
max_attempts: 3
```

The first two failures could return A to the queue with increasing delay:

```text
attempt 1 → NACK → retry after 5 seconds
attempt 2 → NACK → retry after 30 seconds
```

On the third failure, A would move out of the main queue and into its dead-letter queue:

```text
Main queue:
[]

Dead-letter queue:
[A]
```

The dead-letter entry would retain useful failure information:

```json
{
  "message_id":"A",
  "body":"Send email",
  "delivery_attempts":3,
  "last_error":"email provider unavailable"
}
```

An operator could inspect, delete, or manually replay the message after fixing the underlying problem. Each retry and the final dead-letter move would be durable WAL operations.

### 3. WAL snapshots, rotation, and compaction

The current WAL retains every operation, so recovery time and disk usage continue growing even when most historical messages have been removed:

```text
create emails
enqueue A
enqueue B
dequeue A
enqueue C
dequeue C
```

Although only B remains, restart recovery must process the entire history. A snapshot would capture the current state at a known WAL position:

```text
Snapshot at record 1,000:
emails Queue{
    ordering: FIFO,
    sequence: 3,
    messages: [B],
}
```

New operations would continue in a fresh active WAL segment:

```text
record 1,001: enqueue D
record 1,002: dequeue B
```

Restart recovery would become:

```text
load snapshot at record 1,000
→ restore B and sequence 3
→ replay active records 1,001 and 1,002
```

Snapshots improve recovery speed, while older WAL records can still be useful as an audit trail. Rather than immediately deleting them, I would rotate and compress them:

```text
snapshot.json
active.wal
archive/
├── wal-0001.gz
├── wal-0002.gz
└── wal-0003.gz
```

The active WAL remains small, archived segments preserve historical operations, and a configurable retention policy can eventually delete archives when indefinite auditing is unnecessary.

To make snapshot creation crash-safe, the server would write and sync a temporary snapshot, atomically rename it, and only then rotate the covered WAL segment. A crash must always leave either the old recovery history or the new snapshot available.

## Why Choose This over SQS, RabbitMQ, or Pulsar?

### Pros

- One self-contained Go binary
- No external database, broker, or cloud provider
- Simple HTTP API
- Durable priority, FIFO/LIFO, and delayed queues
- Low operational overhead

### Cons

- No replication or high availability
- No ACK/NACK, retries, or dead-letter queues
- No Pub/Sub or advanced routing
- No horizontal scaling
- Limited monitoring and administration tools

This queue is best suited for small systems that value simplicity, local ownership, and minimal infrastructure. Established systems are better choices when scalability, availability, advanced delivery guarantees, and operational tooling are required.

## Current Limitations

The submitted version intentionally does not provide:

- ACK/NACK, visibility timeouts, automatic retries, or dead-letter queues
- Consumer replay after a destructive dequeue
- Blocking dequeue or long polling
- Batch operations
- WAL snapshots, compaction, checksums, or record migrations
- Capacity limits, retention policies, or backpressure
- Authentication, authorization, or tenant isolation
- Replication, clustering, or high availability
- Safe shared access from multiple server processes

Durability protects successfully synced operations from application restarts on one machine. It does not protect against machine or storage-device loss.
