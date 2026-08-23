# HTTP Service

[Repository](../README.md) · [Queue Contract](queue-contract.md) · [Core Queue Engine](core-queue-engine.md) · [Durable Storage](durable-storage.md) · **HTTP Service**

The HTTP layer uses Go's standard library. Handlers decode requests and delegate queue behavior to `service.Manager`.

## Source Navigation

| File | Responsibility |
| --- | --- |
| [`manager.go`](../internal/service/manager.go) | Named queues, durable creation, lookup, and recovery |
| [`handler.go`](../internal/httpapi/handler.go) | Routes, JSON requests, responses, and errors |
| [`main.go`](../cmd/server/main.go) | Startup, configuration, signals, and graceful shutdown |
| [`manager_test.go`](../internal/service/manager_test.go) | Manager creation and restart recovery tests |
| [`handler_test.go`](../internal/httpapi/handler_test.go) | HTTP workflow and validation tests |

## Run the Server

```bash
go run ./cmd/server
```

Defaults:

```text
PORT=8080
WAL_PATH=./data/queue.wal
```

## Endpoints

### Create a queue

```http
POST /queues
Content-Type: application/json

{"name":"emails","ordering":"fifo"}
```

### Inspect a queue

```http
GET /queues/emails
```

### Enqueue a message

```http
POST /queues/emails/messages
Content-Type: application/json

{"body":"Send newsletter","priority":5,"delay_seconds":30}
```

### Dequeue a message

```http
POST /queues/emails/dequeue
```

A successful dequeue returns `200`. If no message is eligible, it returns `204 No Content`.

### Health check

```http
GET /health
```

## Errors

Errors use one shape:

```json
{
  "error": {
    "code": "queue_not_found",
    "message": "queue not found"
  }
}
```

Invalid input returns `400`, missing queues return `404`, duplicate queues return `409`, and storage failures return `500`.
