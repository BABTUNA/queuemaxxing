# Queuemaxxing 💪

An HTTP-based Frankenstein queue supporting configurable FIFO or LIFO ordering, priority, and delayed delivery. The service will own its durable storage implementation, recover after application restarts, and safely support concurrent producers and consumers.

## Documentation

- [Queue Contract](docs/queue-contract.md)
- [Core Queue Engine](docs/core-queue-engine.md)
- [Durable Storage and Recovery](docs/durable-storage.md)
- [HTTP Service](docs/http-service.md)
- [Demonstration Client](docs/demo-client.md)
- [Correctness and Failure Validation](docs/validation.md)

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

**Status:** Implemented in [`internal/client`](internal/client), [`cmd/client`](cmd/client), and [`scripts/demo.sh`](scripts/demo.sh). See [Demonstration Client](docs/demo-client.md).

The demonstration application provides:

- Commands to create, inspect, enqueue, dequeue, and check health
- A polling worker mode for concurrent-consumer demonstrations
- Typed decoding of server API errors
- A repeatable FIFO, LIFO, priority, and delay script
- Client integration tests against the real HTTP handler

## 6. Validate Correctness and Failure Behavior

**Main goal:** Demonstrate that the implementation remains correct under concurrency, crashes, restarts, and malformed persistence data.

**Status:** Validated across the queue, storage, manager, HTTP, and client layers. See [Correctness and Failure Validation](docs/validation.md).

The validation suite covers:

- FIFO, LIFO, priority, delay, and boundary behavior
- Concurrent queue and HTTP operations without duplicate delivery
- Restart recovery and incomplete WAL writes
- Complete WAL corruption detection
- Storage failures through the queue and HTTP boundaries
- Request validation, status codes, and client behavior
- Automated tests, race detection, and vetting in GitHub Actions

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
