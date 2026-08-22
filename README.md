# Queuemaxxing 💪

An HTTP-based Frankenstein queue supporting configurable FIFO or LIFO ordering, priority, and delayed delivery. The service will own its durable storage implementation, recover after application restarts, and safely support concurrent producers and consumers.

## Documentation

- [Queue Contract](docs/queue-contract.md)
- [Core Queue Engine](docs/core-queue-engine.md)

## 1. Establish the Queue's Contract

**Main goal:** Define exactly how the queue behaves before implementation begins. These decisions become the contract enforced by tests and documented for users.

**Status:** Contract defined in [Queue Contract](docs/queue-contract.md).

The contract specifies:

- Priority-first ordering with FIFO or LIFO tie-breaking
- Delay eligibility and promotion behavior
- Destructive, at-most-once dequeue semantics
- Per-queue concurrency guarantees
- Durable mutation and crash-recovery boundaries
- Input defaults, limits, and error behavior

## 2. Build the Core Queue Engine

**Main goal:** Implement FIFO, LIFO, priority, and delay as a standalone Go package without involving HTTP or disk persistence yet.

**Status:** Implemented in [`internal/queue`](internal/queue) with contract-driven unit and concurrency tests. See [Core Queue Engine](docs/core-queue-engine.md).

## 3. Implement Durable Storage and Recovery

**Main goal:** Make queue mutations survive crashes and application restarts without using an external database or queue.

### 3.1 Design the write-ahead log

Use a custom append-only WAL containing records such as:

- Create queue
- Enqueue message
- Dequeue message

Each record should be:

- Length-prefixed
- Checksummed
- Written in a versioned format

### 3.2 Integrate storage with mutations

The mutation sequence will be:

```text
Acquire lock
→ determine mutation
→ append WAL record
→ sync WAL to disk
→ update in-memory state
→ release lock
→ return success
```

If the disk write fails, the in-memory mutation will not be committed.

### 3.3 Implement startup recovery

When the server starts, it will:

1. Open the WAL.
2. Read records in order.
3. Verify each checksum.
4. Replay valid operations.
5. Ignore or truncate an incomplete final record.
6. Reconstruct the ready and delayed heaps.

### 3.4 Plan for future compaction

The initial version may replay the entire WAL. A future version can:

- Write periodic snapshots.
- Rotate old WAL files.
- Remove obsolete records after successful snapshotting.

## 4. Expose the Queue as an HTTP Service

**Main goal:** Wrap the tested queue and persistence layers in a small, predictable HTTP API.

### 4.1 Build the queue manager

The manager will own all named queues:

```go
type Manager struct {
    mu     sync.RWMutex
    queues map[string]*Queue
}
```

It will handle:

- Creating queues
- Looking up queues
- Preventing duplicate queue names
- Coordinating recovery

Each queue will retain its own mutex so unrelated queues can operate concurrently.

### 4.2 Define the initial endpoints

```text
POST /queues
POST /queues/{name}/messages
POST /queues/{name}/dequeue
GET  /queues/{name}
GET  /health
```

### 4.3 Keep handlers thin

Handlers should only:

1. Decode and validate requests.
2. Call the manager or queue engine.
3. Convert results into HTTP responses.
4. Return consistent error objects.

The handlers should not contain ordering or persistence logic.

### 4.4 Add graceful shutdown

On shutdown, the server should:

- Stop accepting new requests.
- Allow active requests to finish.
- Sync and close the WAL.
- Exit cleanly.

## 5. Build the Demonstration Application

**Main goal:** Prove that another application can operate the queue entirely through HTTP.

### 5.1 Implement CLI commands

The client should support commands such as:

```bash
queue-client create emails --ordering fifo
queue-client enqueue emails --body "Newsletter" --priority 1
queue-client enqueue emails --body "Password reset" --priority 10
queue-client dequeue emails
```

### 5.2 Implement worker mode

A worker command will repeatedly dequeue and process messages:

```bash
queue-client worker emails
```

Running multiple workers simultaneously will demonstrate that concurrent consumers do not receive the same message.

### 5.3 Create a repeatable demonstration

The demo should visibly prove:

- Priority ordering
- FIFO and LIFO tie-breaking
- Delayed availability
- Concurrent consumption
- Recovery after restarting the server

## 6. Validate Correctness and Failure Behavior

**Main goal:** Demonstrate that the implementation remains correct under concurrency, crashes, restarts, and malformed persistence data.

### 6.1 Queue unit tests

Cover:

- FIFO
- LIFO
- Priority plus FIFO
- Priority plus LIFO
- Delay plus FIFO/LIFO
- Delay plus priority plus FIFO/LIFO
- Equal availability-time behavior
- Empty queue behavior

### 6.2 Concurrency tests

Test:

- Concurrent producers
- Concurrent consumers
- Mixed enqueue and dequeue
- Multiple named queues
- Duplicate-delivery prevention

Run everything with:

```bash
go test -race ./...
```

### 6.3 Persistence tests

Verify:

- Enqueued messages survive restart.
- Dequeued messages do not reappear.
- Queue configurations survive restart.
- Truncated final WAL records are handled safely.
- Invalid checksums are detected.
- Failed disk writes do not produce successful HTTP responses.

### 6.4 HTTP integration tests

Use `httptest` to verify:

- Request validation
- Status codes
- Response bodies
- Queue-not-found behavior
- Duplicate queue creation
- Concurrent HTTP requests

## 7. Document and Package the Submission

**Main goal:** Make the repository easy to evaluate and show that the engineering tradeoffs were deliberate.

### 7.1 Write the architecture documentation

Explain:

- Package structure
- Ready and delayed heaps
- Ordering algorithm
- Locking strategy
- WAL format
- Recovery process
- Delivery guarantees

### 7.2 Answer the additional questions

Cover:

- How message replay would work
- How the design would evolve into Pub/Sub
- Features that would be added with more time
- Where this queue could be preferable to established products

### 7.3 Document limitations honestly

State that the initial implementation does not yet provide:

- ACK/NACK
- Visibility timeouts
- Automatic retries
- Dead-letter queues
- Replication
- High availability

### 7.4 Prepare the repository

Before submission:

- Add clear setup instructions.
- Include example commands.
- Add an automated demo script if practical.
- Ensure `go test -race ./...` passes.
- Keep runtime data out of Git.
- Tag or create a clean final release.

## Implementation Sequence

```text
Queue contract
→ core engine
→ persistence and recovery
→ HTTP service
→ demo client
→ failure and concurrency testing
→ documentation and submission
```
