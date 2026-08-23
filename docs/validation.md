# Correctness and Failure Validation

[Repository](../README.md) · [Queue Contract](queue-contract.md) · [Core Queue Engine](core-queue-engine.md) · [Durable Storage](durable-storage.md) · [HTTP Service](http-service.md) · [Demo Client](demo-client.md) · **Validation**

The test suite validates behavior at the queue, storage, manager, HTTP, and client boundaries.

## Test Matrix

| Behavior | Primary coverage |
| --- | --- |
| FIFO, LIFO, priority, and delay | [`queue_test.go`](../internal/queue/queue_test.go) |
| Concurrent producers and consumers | [`queue_test.go`](../internal/queue/queue_test.go) |
| Journal failures leave memory unchanged | [`persistence_test.go`](../internal/queue/persistence_test.go) |
| WAL append, replay, partial writes, and corruption | [`wal_test.go`](../internal/storage/wal_test.go) |
| Enqueue/dequeue restart recovery | [`integration_test.go`](../internal/storage/integration_test.go) |
| Named queue recovery and concurrent creation | [`manager_test.go`](../internal/service/manager_test.go) |
| HTTP workflow, validation, and storage failures | [`handler_test.go`](../internal/httpapi/handler_test.go) |
| Unique concurrent HTTP dequeues | [`concurrency_test.go`](../internal/httpapi/concurrency_test.go) |
| Demonstration client workflow and API errors | [`client_test.go`](../internal/client/client_test.go) |

## Failure Boundaries

- A journal failure returns an error before enqueue or dequeue changes memory.
- A storage failure becomes `500 Internal Server Error` through HTTP.
- A partial final WAL line is removed as an interrupted append.
- A complete invalid WAL line stops replay with `ErrInvalidWAL`.
- A final dequeue after concurrent consumption returns `204 No Content`.

## Run Locally

```bash
go test ./...
go test -race ./...
go vet ./...
```

The same commands run in [GitHub Actions](../.github/workflows/test.yml) for pushes and pull requests.

## Deliberate Limits

The first version does not simulate physical power loss, a real disk-full filesystem, multi-process access, replication failure, or long-duration production load. Its deterministic failure tests exercise the application boundaries without requiring external infrastructure.
