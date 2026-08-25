# Queuemaxxing

Queuemaxxing is a durable HTTP queue written in Go. It supports priority-first ordering, FIFO or LIFO tie-breaking, delayed delivery, and concurrent producers and consumers.

The service owns its storage through a local append-only Write-Ahead Log. It does not require an external database, queue service, or message broker.

## Features

- FIFO or LIFO ordering
- Priority-first message selection
- Delayed message availability
- Durable queue creation, enqueue, and dequeue
- Recovery after application restarts
- Concurrent producers and consumers
- HTTP API and demonstration CLI
- No external storage dependencies

## Quick Start

### Requirements

- Go 1.27 or later

### Start the server

```bash
go run ./cmd/server
```

The server starts at `http://localhost:8080` and stores its WAL at `./data/queue.wal`.

### Create a queue

In another terminal:

```bash
go run ./cmd/client create emails --ordering fifo
```

Output:

```json
{
  "name": "emails",
  "ordering": "fifo"
}
```

### Enqueue messages

```bash
go run ./cmd/client enqueue emails \
  --body "Send newsletter" \
  --priority 5

go run ./cmd/client enqueue emails \
  --body "Urgent password reset" \
  --priority 10
```

### Dequeue a message

```bash
go run ./cmd/client dequeue emails
```

The password-reset message is returned first because it has the higher priority.

### Run the complete demonstration

With the server running:

```bash
./scripts/demo.sh
```

The script demonstrates:

- Priority with FIFO ordering
- LIFO ordering
- Delayed message availability

## Queue Behavior

Messages are selected using the following rules:

1. Only messages whose delay has expired are eligible.
2. Higher-priority messages are selected first.
3. Equal-priority messages use the queue's FIFO or LIFO configuration.

Example:

```text
A(priority 5, sequence 1)
B(priority 10, sequence 2)
C(priority 5, sequence 3)
```

FIFO dequeue order:

```text
B → A → C
```

LIFO dequeue order:

```text
B → C → A
```

A delayed message does not participate in ordering until its `AvailableAt` timestamp is reached.

## Delivery Model

Dequeue is destructive and provides at-most-once delivery.

```text
select message
→ write and sync dequeue record
→ remove message from memory
→ return response
```

There is no acknowledgement step, visibility timeout, or automatic retry. If the dequeue record is committed but the response does not reach the consumer, the message remains removed.

## Architecture

```text
HTTP request
→ HTTP handler
→ Queue manager
→ Named queue
→ Queue journal
→ WAL file
```

Each queue contains:

- A ready heap for immediately eligible messages
- A delayed heap ordered by availability time
- A mutex protecting its sequence and both heaps
- A journal that records mutations before memory changes

The manager maps queue names to queue instances and coordinates durable queue creation.

## Durability and Recovery

Every successful mutation is written and synced before the in-memory queue changes:

```text
lock queue
→ append WAL record
→ sync WAL
→ update memory
→ unlock
→ respond
```

The WAL contains JSON-lines records such as:

```json
{"operation":"create_queue","queue":"emails","ordering":"fifo"}
{"operation":"enqueue","queue":"emails","message":{"id":"A"}}
{"operation":"dequeue","queue":"emails","message_id":"A"}
```

During startup, the server replays these records to reconstruct queue definitions, sequence numbers, and undequeued messages.

An incomplete final record is treated as an interrupted write and removed. A complete invalid record is treated as corruption and stops startup.

## HTTP API

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Check server health |
| `POST` | `/queues` | Create a queue |
| `GET` | `/queues/{name}` | Inspect a queue |
| `POST` | `/queues/{name}/messages` | Enqueue a message |
| `POST` | `/queues/{name}/dequeue` | Dequeue the next eligible message |

### Create a queue

```http
POST /queues
Content-Type: application/json

{"name":"emails","ordering":"fifo"}
```

### Enqueue a message

```http
POST /queues/emails/messages
Content-Type: application/json

{
  "body": "Send newsletter",
  "priority": 5,
  "delay_seconds": 30
}
```

### Dequeue a message

```http
POST /queues/emails/dequeue
```

A successful dequeue returns `200 OK`. If no message is eligible, the server returns `204 No Content`.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP server port |
| `WAL_PATH` | `./data/queue.wal` | Durable WAL location |
| `QUEUE_URL` | `http://localhost:8080` | Server URL used by the CLI |

Example:

```bash
PORT=9000 WAL_PATH=./data/custom.wal go run ./cmd/server
```

Then connect the client:

```bash
QUEUE_URL=http://localhost:9000 go run ./cmd/client health
```

## Testing

Run the full test suite:

```bash
go test ./...
```

Run concurrency tests with Go's race detector:

```bash
go test -race ./...
```

Run static analysis:

```bash
go vet ./...
```

The test suite covers:

- FIFO, LIFO, priority, and delay behavior
- Concurrent enqueue and dequeue operations
- WAL append and restart recovery
- Partial and corrupt WAL records
- Storage failures
- HTTP validation and error responses
- Demonstration-client integration

The same commands run through GitHub Actions on pushes and pull requests.

## Project Structure

```text
cmd/server          HTTP server entry point
cmd/client          Demonstration CLI
internal/queue      Core queue engine and heaps
internal/storage    WAL, journal, and recovery
internal/service    Named queue manager
internal/httpapi    HTTP routes and responses
internal/client     Reusable HTTP client
scripts             Repeatable demonstration
docs                Architecture and design documentation
```

## Documentation

- [Queue Contract](docs/queue-contract.md)
- [Core Queue Engine](docs/core-queue-engine.md)
- [Durable Storage and Recovery](docs/durable-storage.md)
- [HTTP Service](docs/http-service.md)
- [Demonstration Client](docs/demo-client.md)
- [Correctness and Failure Validation](docs/validation.md)
- [Design Decisions and Future Work](docs/design-decisions.md)

The design-decisions document covers:

- Message replay and restart recovery
- Refactoring the queue into Pub/Sub
- Features that would be added with more time
- Advantages and limitations compared with established systems

## Current Limitations

- No ACK/NACK, visibility timeouts, retries, or dead-letter queues
- No Pub/Sub or advanced routing
- No WAL snapshots or compaction
- No replication or high availability
- No authentication or tenant isolation
- One server process must own each WAL
