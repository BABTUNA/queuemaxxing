# Demonstration Client

[Repository](../README.md) · [Queue Contract](queue-contract.md) · [Core Queue Engine](core-queue-engine.md) · [Durable Storage](durable-storage.md) · [HTTP Service](http-service.md) · **Demo Client**

The demonstration application is a small command-line client for the queue's HTTP API.

## Source Navigation

| File | Responsibility |
| --- | --- |
| [`client.go`](../internal/client/client.go) | HTTP requests, responses, and API errors |
| [`main.go`](../cmd/client/main.go) | Commands, flags, output, and worker mode |
| [`client_test.go`](../internal/client/client_test.go) | End-to-end client tests using the real handler in memory |
| [`demo.sh`](../scripts/demo.sh) | Repeatable FIFO, LIFO, priority, and delay demonstration |

## Start the Server

```bash
go run ./cmd/server
```

The client uses `http://localhost:8080` by default. Override it with:

```bash
QUEUE_URL=http://localhost:9000 go run ./cmd/client health
```

## Commands

```bash
go run ./cmd/client create emails --ordering fifo
go run ./cmd/client get emails
go run ./cmd/client enqueue emails --body "Newsletter" --priority 5
go run ./cmd/client enqueue emails --body "Later" --delay 30
go run ./cmd/client dequeue emails
go run ./cmd/client health
```

## Worker Mode

Continuously consume available messages:

```bash
go run ./cmd/client worker emails --interval 1s
```

Run the command in multiple terminals to demonstrate concurrent consumption. Press `Ctrl+C` to stop a worker.

## Repeatable Demo

With the server running in another terminal:

```bash
./scripts/demo.sh
```

The script demonstrates priority-first FIFO ordering, LIFO ordering, and delayed availability. It uses unique queue names so it can be run repeatedly.

## Restart Recovery

1. Create a queue and enqueue messages with the client.
2. Stop the server with `Ctrl+C`.
3. Start the server again with the same `WAL_PATH`.
4. Dequeue the messages with the client.

The messages remain available because acknowledged mutations were synced to the WAL.
