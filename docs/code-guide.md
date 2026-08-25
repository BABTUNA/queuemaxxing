# Queuemaxxing Code Guide

This guide gives you a dependency-first understanding of the repository. Lower-tier files define the pieces that higher-tier files use.

For this guide, a **project dependency** means one production file relies on a type or function defined in another production file in this repository. Standard-library packages such as `time`, `errors`, and `net/http` do not raise a file's tier.

## Project Overview

Queuemaxxing has two main operating flows.

### Normal operation

```text
CLI command
→ HTTP request
→ HTTP handler
→ queue manager
→ named queue
→ queue journal
→ WAL file
```

The queue engine decides message ordering. The journal and WAL make each accepted mutation durable.

### Restart recovery

```text
WAL file
→ replay records
→ recover queue states
→ restore named queues
→ rebuild ready and delayed heaps
→ start HTTP server
```

## File Tiers

Tier 0 is the foundation. Each higher tier uses definitions or behavior from lower tiers.

### Tier 0: Fundamental Definitions

These files do not rely on another project file.

| File | Defines | Purpose | Used by |
| --- | --- | --- | --- |
| [`internal/queue/message.go`](../internal/queue/message.go) | Ordering modes, message data, enqueue input, recovery state | Provide the shared data model passed between every queue layer | Queue engine ([`queue.go`](../internal/queue/queue.go)); heaps ([`ready_heap.go`](../internal/queue/ready_heap.go), [`delayed_heap.go`](../internal/queue/delayed_heap.go)); storage ([`record.go`](../internal/storage/record.go), [`recovery.go`](../internal/storage/recovery.go)); manager ([`manager.go`](../internal/service/manager.go)); HTTP client ([`client.go`](../internal/client/client.go)) |
| [`internal/queue/errors.go`](../internal/queue/errors.go) | Stable queue validation and sequence errors | Let callers recognize queue failures without comparing error text | Queue engine ([`queue.go`](../internal/queue/queue.go)); HTTP error mapping ([`handler.go`](../internal/httpapi/handler.go)); queue tests ([`queue_test.go`](../internal/queue/queue_test.go)) |

Detailed guides currently generated:

- [`queue-message.md`](code-guide/queue-message.md)
- [`queue-errors.md`](code-guide/queue-errors.md)

### Tier 1: Mechanics Built from Fundamental Types

These files depend primarily on Tier 0 definitions.

| File | Depends on | Purpose |
| --- | --- | --- |
| [`internal/queue/ready_heap.go`](../internal/queue/ready_heap.go) | `Message`, `Ordering` ([`message.go`](../internal/queue/message.go)) | Priority plus FIFO/LIFO ranking |
| [`internal/queue/delayed_heap.go`](../internal/queue/delayed_heap.go) | `Message` ([`message.go`](../internal/queue/message.go)) | Earliest-availability ranking |
| [`internal/queue/journal.go`](../internal/queue/journal.go) | `Message` ([`message.go`](../internal/queue/message.go)) | Persistence interface and memory-only implementation |
| [`internal/storage/record.go`](../internal/storage/record.go) | `Ordering`, `Message` ([`message.go`](../internal/queue/message.go)) | Create, enqueue, and dequeue WAL records |
| [`internal/client/client.go`](../internal/client/client.go) | `Ordering`, `Message` ([`message.go`](../internal/queue/message.go)) | Reusable HTTP client and response models |

Detailed file guides are intentionally not generated for this tier yet.

### Tier 2: Core Behavior

These files combine fundamental types with Tier 1 mechanics.

| File | Depends on | Purpose |
| --- | --- | --- |
| [`internal/queue/queue.go`](../internal/queue/queue.go) | Queue types ([`queue/message.go`](../internal/queue/message.go)); errors ([`queue/errors.go`](../internal/queue/errors.go)); heaps ([`queue/ready_heap.go`](../internal/queue/ready_heap.go), [`queue/delayed_heap.go`](../internal/queue/delayed_heap.go)); queue journal interface ([`queue/journal.go`](../internal/queue/journal.go)) | Enqueue, dequeue, delay promotion, restoration |
| [`internal/storage/wal.go`](../internal/storage/wal.go) | Storage records ([`record.go`](../internal/storage/record.go)) | Append, sync, replay, and partial-write handling |
| [`internal/storage/recovery.go`](../internal/storage/recovery.go) | Storage records ([`record.go`](../internal/storage/record.go)); queue state ([`message.go`](../internal/queue/message.go)) | Convert WAL history into recoverable queue states |
| [`cmd/client/main.go`](../cmd/client/main.go) | Client package ([`client.go`](../internal/client/client.go)); queue limits and ordering ([`message.go`](../internal/queue/message.go)) | CLI commands, flags, output, and worker mode |

### Tier 3: Queue-to-WAL Adapter

| File | Depends on | Purpose |
| --- | --- | --- |
| [`internal/storage/journal.go`](../internal/storage/journal.go) | Queue journal interface ([`queue/journal.go`](../internal/queue/journal.go)); WAL ([`wal.go`](../internal/storage/wal.go)); records ([`record.go`](../internal/storage/record.go)) | Connect one named queue to the WAL |

### Tier 4: Queue and Storage Coordination

| File | Depends on | Purpose |
| --- | --- | --- |
| [`internal/service/manager.go`](../internal/service/manager.go) | Queue engine ([`queue.go`](../internal/queue/queue.go)); WAL ([`wal.go`](../internal/storage/wal.go)); recovery ([`recovery.go`](../internal/storage/recovery.go)); storage journal ([`storage/journal.go`](../internal/storage/journal.go)) | Own named queues, durable creation, recovery, lookup |

### Tier 5: HTTP Application Layer

| File | Depends on | Purpose |
| --- | --- | --- |
| [`internal/httpapi/handler.go`](../internal/httpapi/handler.go) | Manager ([`manager.go`](../internal/service/manager.go)); queue types ([`message.go`](../internal/queue/message.go)); queue errors ([`errors.go`](../internal/queue/errors.go)) | Routes, JSON decoding, responses, HTTP errors |

### Tier 6: Server Entry Point

| File | Depends on | Purpose |
| --- | --- | --- |
| [`cmd/server/main.go`](../cmd/server/main.go) | Storage ([`wal.go`](../internal/storage/wal.go)); manager ([`manager.go`](../internal/service/manager.go)); HTTP handler ([`handler.go`](../internal/httpapi/handler.go)) | Startup, recovery, serving, graceful shutdown |

Test files sit beside the production layer they validate; they are not assigned architecture tiers.

## Companion Learning Guides

- [`traces.md`](code-guide/traces.md) follows core operations across files and tiers, including creation, enqueue, dequeue, failure, and restart recovery.
- [`core-concepts.md`](code-guide/core-concepts.md) explains the project-specific structures used to represent messages, queues, persistence, and recovery.
- [`function-index.md`](code-guide/function-index.md) catalogs every production function by file, purpose, input/output shape, relationships, size, and importance.

## How to Review a File Guide

Each individual file guide follows the same order:

1. Why the file exists
2. What it depends on
3. What depends on it
4. Definitions in the file
5. Concrete input/output examples
6. Walkthroughs showing how its definitions move through the application

Only the two Tier 0 guides exist during this experiment. If this structure works, guides for Tier 1 should be generated next, followed by higher tiers.
