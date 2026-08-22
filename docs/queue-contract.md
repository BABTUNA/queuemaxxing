# Queue Contract

This document defines the externally observable behavior of Queuemaxxing. The implementation and its tests must conform to this contract. Changes to these semantics should be deliberate and documented.

## 1. Queue Configuration

### Goal

Give every queue one stable, unambiguous ordering policy for its lifetime.

### 1.1 Queue identity

- A queue name is unique within one server data directory.
- Names must be between 1 and 64 characters.
- Names must match `[a-z0-9][a-z0-9_-]{0,63}`.
- Queue names are case-sensitive by construction because uppercase characters are invalid.
- Creating an existing queue returns a conflict and does not alter its configuration.

### 1.2 Ordering mode

- A queue is created with exactly one ordering mode: `fifo` or `lifo`.
- Ordering mode is required when the queue is created.
- Ordering mode cannot be changed after creation.
- A different ordering mode requires a new queue.

## 2. Message Contract

### Goal

Represent enough information to produce deterministic ordering, delay eligibility, persistence, and recovery.

### 2.1 Message fields

Every accepted message has:

- `id`: an opaque, server-generated unique identifier.
- `body`: a non-empty UTF-8 string containing the user payload.
- `priority`: a signed 32-bit integer.
- `sequence`: a server-generated unsigned 64-bit integer scoped to the queue.
- `created_at`: the server's UTC enqueue time.
- `available_at`: the first UTC instant at which the message may be dequeued.

Timestamps are represented externally as RFC 3339 strings with nanosecond precision.

### 2.2 Defaults and limits

- Omitted priority defaults to `0`.
- Higher integers mean higher priority; negative priorities are valid.
- Omitted delay defaults to `0` seconds.
- Delay must be an integer from `0` through `900` seconds, inclusive.
- Message bodies must contain at least one byte and no more than 256 KiB after UTF-8 encoding.
- IDs, timestamps, and sequence numbers are assigned by the server and cannot be supplied by clients.

The 15-minute delay ceiling intentionally matches the basic delay range exposed by Amazon SQS while keeping the first implementation bounded.

## 3. Ordering Semantics

### Goal

Produce one deterministic next message for every set of eligible messages.

### 3.1 Priority is primary

Among eligible messages, the message with the highest numeric priority is selected first.

```text
A(priority=1)
B(priority=10)
C(priority=5)

dequeue order: B, C, A
```

### 3.2 FIFO tie-breaking

For a `fifo` queue, the lowest sequence number wins when priorities are equal.

```text
A(priority=5, sequence=1)
B(priority=5, sequence=2)

dequeue order: A, B
```

### 3.3 LIFO tie-breaking

For a `lifo` queue, the highest sequence number wins when priorities are equal.

```text
A(priority=5, sequence=1)
B(priority=5, sequence=2)

dequeue order: B, A
```

### 3.4 Sequence assignment

- Sequence numbers increase monotonically within each queue.
- A sequence number is assigned when enqueue begins and is persisted with the message.
- Recovery restores the next sequence number above every previously assigned sequence.
- Sequence numbers are never reused, including after a message is dequeued.
- Sequence exhaustion causes enqueue to fail rather than wrap to zero.

## 4. Delay Semantics

### Goal

Keep delayed messages out of ready-message ordering until their delays expire.

### 4.1 Availability calculation

The server calculates availability once during enqueue:

```text
available_at = created_at + delay_seconds
```

Clients cannot submit an absolute availability timestamp.

### 4.2 Eligibility boundary

A message is eligible when:

```text
available_at <= dequeue_time
```

It is ineligible for every earlier instant.

### 4.3 Interaction with priority

Ineligible messages do not participate in ready-message ordering, regardless of priority.

```text
A(priority=100, delay=60s)
B(priority=1, delay=0s)

Before 60 seconds: B is available.
At or after 60 seconds: A is available.
```

When a delayed message becomes eligible, it retains its original priority and enqueue sequence. Its original sequence therefore participates in FIFO/LIFO tie-breaking.

### 4.4 Promotion behavior

- The implementation may store delayed and ready messages separately.
- Before selecting a message, dequeue must promote every delayed message eligible at that operation's captured time.
- Correctness cannot depend on a background scheduler running at an exact instant.
- An eligible message may remain physically stored in the delayed structure until the next queue operation, but it must behave as ready during dequeue.

## 5. Delivery Semantics

### Goal

Provide a small, explicit first delivery model without implying acknowledgement guarantees that are not implemented.

### 5.1 Destructive dequeue

- Dequeue permanently removes the selected message.
- A successful dequeue returns the complete message.
- Dequeue does not create a lease and does not require acknowledgement.
- If no message is eligible, the HTTP API returns `204 No Content`.

### 5.2 Delivery guarantee

The initial implementation provides at-most-once delivery:

- A successfully committed dequeue never returns the same message again.
- If the server crashes after durably committing a dequeue but before the client receives the response, the message remains removed.
- Clients cannot determine from a lost response whether that dequeue committed.

ACK/NACK, visibility timeouts, automatic redelivery, and dead-letter queues are explicitly outside the first version.

## 6. Concurrency Semantics

### Goal

Make operations safe and predictable when multiple producers and consumers use the service simultaneously.

### 6.1 Per-queue atomicity

- Enqueue and dequeue are atomic with respect to other operations on the same queue.
- Operations on one queue appear to execute in one serial order.
- Two successful concurrent dequeues cannot return the same message.
- Sequence numbers remain unique and increasing during concurrent enqueue operations.

### 6.2 Cross-queue behavior

- Queues have independent ordering and sequence spaces.
- Operations on different queues may execute concurrently.
- No ordering guarantee exists across different queues.

### 6.3 Process ownership

- Exactly one server process may own a data directory at a time.
- Starting a second process against an in-use data directory must fail rather than risk corruption.
- Multi-process clustering and replication are not part of the first version.

## 7. Durability and Recovery Semantics

### Goal

Ensure acknowledged mutations survive application restarts and make corruption behavior explicit.

### 7.1 Successful mutation boundary

Create, enqueue, and dequeue operations are successful only after their WAL record has been written and synced to disk.

```text
acquire lock
→ determine mutation
→ append WAL record
→ sync WAL
→ update in-memory state
→ release lock
→ return success
```

If appending or syncing fails:

- The operation returns an internal error.
- The corresponding in-memory mutation is not committed.
- The server does not claim the operation succeeded.

### 7.2 Restart recovery

On startup, the server replays valid WAL records in order to reconstruct:

- Queue definitions
- Undequeued messages
- Ready and delayed membership
- The next sequence number for every queue

After recovery, externally observable behavior must match behavior immediately before shutdown for every acknowledged operation.

### 7.3 Incomplete and corrupt records

- An incomplete final WAL record is treated as an interrupted write and ignored or truncated.
- A checksum failure in a complete record is treated as corruption.
- Corruption before the incomplete tail causes startup to fail loudly.
- The server must not silently skip an interior corrupt record and continue with potentially inconsistent state.

### 7.4 Durability boundary

The first version targets durability on one machine using successful file synchronization. It does not claim resilience against simultaneous loss of the machine and its storage, faulty storage hardware, or filesystem behavior that violates successful sync guarantees.

## 8. Validation and Error Behavior

### Goal

Make invalid operations fail consistently without changing durable or in-memory state.

| Condition | HTTP result | Mutation |
| --- | --- | --- |
| Invalid queue name | `400 Bad Request` | None |
| Invalid ordering mode | `400 Bad Request` | None |
| Invalid body, priority, or delay | `400 Bad Request` | None |
| Queue already exists | `409 Conflict` | None |
| Queue does not exist | `404 Not Found` | None |
| No eligible message | `204 No Content` | None |
| WAL append or sync failure | `500 Internal Server Error` | None |

Error responses use a stable JSON shape:

```json
{
  "error": {
    "code": "invalid_delay",
    "message": "delay_seconds must be between 0 and 900"
  }
}
```

## 9. Contract Acceptance Cases

### Goal

Define the minimum behavioral examples that Section 2 tests must encode.

| Queue | Enqueued messages | Expected dequeue order |
| --- | --- | --- |
| FIFO | `A, B, C` | `A, B, C` |
| LIFO | `A, B, C` | `C, B, A` |
| Priority + FIFO | `A(p5), B(p10), C(p5)` | `B, A, C` |
| Priority + LIFO | `A(p5), B(p10), C(p5)` | `B, C, A` |
| Delay + FIFO | `A(d10), B(d0)` | `B`, then `A` after eligibility |
| Delay + LIFO | `A(d10), B(d0)` | `B`, then `A` after eligibility |
| Delay + Priority + FIFO | `A(p10,d10), B(p1,d0), C(p10,d10)` | `B`, then `A, C` once both delayed messages are eligible |
| Delay + Priority + LIFO | `A(p10,d10), B(p1,d0), C(p10,d10)` | `B`, then `C, A` once both delayed messages are eligible |

Additional required cases:

- A message is unavailable one instant before `available_at` and available exactly at `available_at`.
- Concurrent successful dequeues return distinct message IDs.
- Restart recovery preserves queue configuration and expected dequeue order.
- A truncated final WAL record does not resurrect or invent an acknowledged operation.

## 10. First-Version Non-Goals

### Goal

Prevent adjacent distributed-systems features from obscuring the required queue implementation.

The first version does not include:

- Acknowledgements or negative acknowledgements
- Visibility timeouts or automatic retries
- Dead-letter queues
- Blocking dequeue or long polling
- Pub/Sub fan-out
- Replication or high availability
- Multiple server processes sharing one data directory
- WAL snapshots or compaction
- Authentication, authorization, or tenant isolation
- Per-queue capacity limits or backpressure
