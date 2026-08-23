# Queuemaxxing 💪

An HTTP-based Frankenstein queue supporting configurable FIFO or LIFO ordering, priority, and delayed delivery. The service will own its durable storage implementation, recover after application restarts, and safely support concurrent producers and consumers.

## Documentation

- [Queue Contract](docs/queue-contract.md)
- [Core Queue Engine](docs/core-queue-engine.md)
- [Durable Storage and Recovery](docs/durable-storage.md)
- [HTTP Service](docs/http-service.md)

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

The engine implements:

- Priority-first ordering with FIFO or LIFO tie-breaking
- Separate ready and delayed heaps
- Delay eligibility and promotion during dequeue
- Message validation, sequence assignment, and UUIDv4 IDs
- Mutex-protected enqueue and dequeue operations
- Contract-driven unit, boundary, and concurrency tests

## 3. Implement Durable Storage and Recovery

**Main goal:** Make queue mutations survive crashes and application restarts without using an external database or queue.

**Status:** Implemented in [`internal/storage`](internal/storage) and integrated through the queue journal boundary. See [Durable Storage and Recovery](docs/durable-storage.md).

The storage layer implements:

- Append-only JSON-lines WAL records
- Durable create, enqueue, and dequeue operations
- Sync-before-memory mutation ordering
- Incomplete final-record recovery
- Queue-state replay and ready/delayed heap restoration
- Restart and write-failure tests

## 4. Expose the Queue as an HTTP Service

**Main goal:** Wrap the tested queue and persistence layers in a small, predictable HTTP API.

**Status:** Implemented in [`internal/service`](internal/service), [`internal/httpapi`](internal/httpapi), and [`cmd/server`](cmd/server). See [HTTP Service](docs/http-service.md).

The HTTP service implements:

- Durable creation and recovery of named queues
- Create, inspect, enqueue, dequeue, and health endpoints
- Strict JSON decoding and contract-aligned errors
- Independent concurrency across named queues
- Standard-library routing and graceful shutdown
- Manager and HTTP workflow tests

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
- Invalid complete JSON records are detected.
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
