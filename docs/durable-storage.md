# Durable Storage and Recovery

[Repository](../README.md) · [Queue Contract](queue-contract.md) · [Core Queue Engine](core-queue-engine.md) · **Durable Storage**

The storage layer owns one append-only Write-Ahead Log (WAL). It does not use a database or another queue.

## Source Navigation

| File | Responsibility |
| --- | --- |
| [`record.go`](../internal/storage/record.go) | Create, enqueue, and dequeue record types |
| [`wal.go`](../internal/storage/wal.go) | JSON-lines append, sync, and replay |
| [`journal.go`](../internal/storage/journal.go) | Connects a named queue to the WAL |
| [`recovery.go`](../internal/storage/recovery.go) | Rebuilds queue state from records |
| [`wal_test.go`](../internal/storage/wal_test.go) | Append, replay, and partial-write tests |
| [`integration_test.go`](../internal/storage/integration_test.go) | Restart recovery test |

## Record Format

Each operation is one JSON line:

```json
{"operation":"create_queue","queue":"emails","ordering":"fifo"}
{"operation":"enqueue","queue":"emails","message":{"id":"..."}}
{"operation":"dequeue","queue":"emails","message_id":"..."}
```

## Durable Mutations

The queue records each mutation before changing memory:

```text
lock queue
→ append WAL record
→ sync WAL file
→ update memory
→ unlock
```

If the append or sync fails, the queue returns an error and its in-memory state remains unchanged.

## Restart Recovery

At startup, `WAL.Replay` reads complete JSON lines in order. A partial final line is treated as an interrupted write and removed. `storage.Recover` rebuilds each queue's ordering, sequence number, and undequeued messages. `queue.Restore` places those messages back into the ready or delayed heap.

The initial implementation expects one server process per WAL. Checksums, file locking, snapshots, and compaction are intentionally left for future work.

## Verification

```bash
go test ./...
go test -race ./...
go vet ./...
```
