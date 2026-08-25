# Production Function Index

[Back to Code Guide](../code-guide.md) · [Core Structures](core-concepts.md) · [Detailed Traces](traces.md)

This index helps choose which functions deserve a deeper walkthrough. It covers production functions only and is organized by dependency tier, then source file.

## How to Read the Index

**Size** counts the function signature and body, including blank lines inside the function but excluding its preceding comment. It is a navigation aid, not a quality score.

**Importance** measures how much understanding the function contributes to the system:

| Rating | Meaning |
| --- | --- |
| **Critical** | Controls queue state, durability, recovery, or concurrency correctness |
| **High** | Owns an important application boundary or coordinates a core operation |
| **Supporting** | Implements a meaningful mechanic used by a more important function |
| **Trivial** | Thin delegation, accessor, formatting, or configuration helper |

Input/output entries use concrete Go types but omit receiver values unless the receiver is important to understanding the result.

## Tier 0: Fundamental Definitions

### [`internal/queue/message.go`](../../internal/queue/message.go)

Purpose: Defines the shared queue data model—`Ordering`, `Message`, `EnqueueInput`, `State`, and the input limits used by higher tiers.

This file contains no functions. Its structures are covered in [Core Project Structures](core-concepts.md).

### [`internal/queue/errors.go`](../../internal/queue/errors.go)

Purpose: Defines stable sentinel errors returned by queue validation, construction, and sequence assignment.

This file contains no functions. Callers use these errors with `errors.Is`.

## Tier 1: Mechanics and Boundary Types

### [`internal/queue/ready_heap.go`](../../internal/queue/ready_heap.go)

Purpose: Implements the heap contract for messages that are currently eligible. Its comparator combines priority-first ordering with FIFO/LIFO sequence tie-breaking.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `readyHeap.Len` | Reports the number of ready messages so `container/heap` can maintain the structure and `Queue.Dequeue` can detect an empty queue. | heap → `int` | Called by `container/heap`, `Queue.Dequeue`; no project calls | 3 | Supporting |
| `readyHeap.Less` | Defines which of two eligible messages ranks closer to the heap root: higher priority wins, then lower sequence for FIFO or higher sequence for LIFO. | indexes `i, j` → `bool` | Called by `container/heap`; reads `Message.Priority`, `Sequence`, and heap ordering | 14 | **Critical** |
| `readyHeap.Swap` | Exchanges two underlying slice positions when the standard heap algorithm repairs ordering after a push or pop. | indexes `i, j` → no return | Called by `container/heap`; swaps `items[i]` and `items[j]` | 3 | Supporting |
| `readyHeap.Push` | Appends a newly eligible `Message` to the underlying slice; `container/heap` subsequently moves it to the correct position. | `any` containing `Message` → no return | Called by `heap.Push` from enqueue, promotion, and restore | 3 | Supporting |
| `readyHeap.Pop` | Removes and returns the slice element that `container/heap` has already moved to the end, then clears its slot to release references. | heap → `any` containing `Message` | Called by `heap.Pop` from `Queue.Dequeue` | 7 | Supporting |

### [`internal/queue/delayed_heap.go`](../../internal/queue/delayed_heap.go)

Purpose: Implements the heap contract for messages that are not eligible yet, keeping the next availability deadline at the root.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `delayedHeap.Len` | Reports how many delayed messages exist so promotion and the standard heap algorithm can determine whether work remains. | heap → `int` | Called by `container/heap`, `promoteEligible` | 3 | Supporting |
| `delayedHeap.Less` | Ranks earlier `AvailableAt` timestamps first and uses lower sequence as a deterministic tie-breaker when two deadlines match. | indexes `i, j` → `bool` | Called by `container/heap`; compares delayed `Message` values | 7 | **Critical** |
| `delayedHeap.Swap` | Exchanges delayed-message positions while `container/heap` restores heap invariants. | indexes `i, j` → no return | Called by `container/heap` | 3 | Supporting |
| `delayedHeap.Push` | Appends a future message to the delayed slice before the standard heap algorithm positions it by availability time. | `any` containing `Message` → no return | Called by `heap.Push` from enqueue and restore | 3 | Supporting |
| `delayedHeap.Pop` | Returns and removes the end element prepared by `container/heap`, clearing the old slot before shrinking the slice. | heap → `any` containing `Message` | Called by `heap.Pop` from `promoteEligible` | 8 | Supporting |

### [`internal/queue/journal.go`](../../internal/queue/journal.go)

Purpose: Defines the persistence operations the queue engine requires and supplies a memory-only implementation for queues created without a WAL.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `noopJournal.RecordEnqueue` | Satisfies the enqueue persistence hook without writing anything, allowing `queue.New` to use the same mutation path as durable queues. | `Message` → `nil` | Called through `Journal` by `Queue.Enqueue`; calls nothing | 3 | Supporting |
| `noopJournal.RecordDequeue` | Satisfies the dequeue persistence hook without writing anything for an explicitly memory-only queue. | message ID → `nil` | Called through `Journal` by `Queue.Dequeue`; calls nothing | 3 | Supporting |

The `Journal` interface itself is not a function, but it is a **Critical** boundary because queue mutations depend on its success before changing memory.

### [`internal/storage/record.go`](../../internal/storage/record.go)

Purpose: Defines the three durable operation shapes written to the WAL and validates that each operation carries the fields recovery requires.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `NewCreateQueueRecord` | Packages a queue name and FIFO/LIFO mode into the durable operation needed to recreate that queue after restart. | `string, queue.Ordering` → `Record` | Called by `Manager.CreateQueue`; calls nothing | 3 | High |
| `NewEnqueueRecord` | Packages the queue name and complete accepted message so replay can reconstruct all message metadata and ordering fields. | `string, queue.Message` → `Record` | Called by `QueueJournal.RecordEnqueue`; calls nothing | 3 | High |
| `NewDequeueRecord` | Packages the queue name and removed message ID so replay can delete that message from recovered state. | two `string` values → `Record` | Called by `QueueJournal.RecordDequeue`; calls nothing | 3 | High |
| `Record.validate` | Rejects structurally incomplete or unknown WAL operations before append and again during replay, preventing invalid records from becoming trusted state. | `Record` → `error` | Called by `WAL.Append`, `WAL.Replay`, `Recover` | 22 | **Critical** |

### [`internal/client/client.go`](../../internal/client/client.go)

Purpose: Implements the reusable HTTP client used by the demonstration CLI, including request serialization, response limits, status handling, and API error decoding.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `APIError.Error` | Converts a structured server error into readable CLI text, preserving the API code when one was decoded. | `APIError` → `string` | Called by Go error formatting; calls `fmt.Sprintf` when code is absent | 6 | Trivial |
| `New` | Normalizes the server URL and creates a client with a ten-second HTTP timeout so every command shares consistent transport behavior. | base URL `string` → `*Client` | Called by CLI `run`; constructs `http.Client` | 8 | Supporting |
| `Client.CreateQueue` | Builds the create-queue payload and delegates the POST request, decoding the successful response into client `QueueInfo`. | context, name, ordering → `QueueInfo, error` | Called by `runCreate`; calls `Client.do` | 8 | High |
| `Client.GetQueue` | Escapes the queue name, issues a GET request, and decodes the queue metadata response. | context, name → `QueueInfo, error` | Called by `runGet`; calls `Client.do` | 5 | Supporting |
| `Client.Enqueue` | Converts the client enqueue model into an HTTP POST and decodes the server-assigned `queue.Message`. | context, name, `client.EnqueueInput` → `queue.Message, error` | Called by `runEnqueue`; calls `Client.do` | 5 | High |
| `Client.Dequeue` | Issues a dequeue POST and distinguishes an empty `204` response from a returned message, exposing that distinction as the `bool` result. | context, name → `Message, bool, error` | Called by `runDequeue`, `runWorker`; calls `Client.request` | 11 | High |
| `Client.Health` | Calls the health endpoint and verifies the decoded status is exactly `"ok"`, rather than treating any JSON success as healthy. | context → `error` | Called by CLI `run`; calls `Client.do` | 12 | Supporting |
| `Client.do` | Provides the common error-only wrapper used when callers do not need the HTTP status code. | request components + destination → `error` | Called by create/get/enqueue/health; calls `Client.request` | 4 | Trivial |
| `Client.request` | Owns the complete client transport boundary: JSON encoding, contextual request creation, HTTP execution, response-size limiting, non-2xx error conversion, and success decoding. | context, method, path, optional body/destination → status `int, error` | Called by `Client.do`, `Client.Dequeue`; calls HTTP and `decodeAPIError` | 36 | **Critical** |
| `decodeAPIError` | Decodes the server's structured error envelope and falls back to standard HTTP status text when the body is malformed or incomplete. | status, response body → `error` | Called by `Client.request`; constructs `APIError` | 16 | Supporting |

## Tier 2: Core Behavior

### [`internal/queue/queue.go`](../../internal/queue/queue.go)

Purpose: Owns the queue's live state and implements construction, restoration, validation, durable enqueue/dequeue, delay promotion, and ID/sequence assignment.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `New` | Validates FIFO/LIFO mode and creates an empty memory-only queue using the no-op journal and production ID generator. | `Ordering` → `*Queue, error` | Called by core tests/package consumers; calls `newQueue` | 7 | Supporting |
| `NewWithJournal` | Validates ordering and the persistence dependency before constructing a queue whose future mutations must be journaled. | `Ordering, Journal` → `*Queue, error` | Called by `Manager.CreateQueue`, `Restore`; calls `newQueue` | 10 | High |
| `newQueue` | Centralizes field wiring for both constructors, ensuring the ready heap receives the same ordering mode as the queue and dependencies are stored consistently. | ordering, ID generator, journal → `*Queue` | Called by `New`, `NewWithJournal`; constructs queue state | 10 | Supporting |
| `Restore` | Reconstructs a live queue from durable state without rewriting history, restores its sequence counter, and divides surviving messages between ready and delayed heaps using `now`. | `State, time.Time, Journal` → `*Queue, error` | Called by `NewManager`; calls `NewWithJournal`, `heap.Push` | 18 | **Critical** |
| `Queue.Ordering` | Exposes the immutable ordering mode needed to describe a queue through the manager and HTTP API. | queue receiver → `Ordering` | Called by `Manager.Queue`; calls nothing | 3 | Trivial |
| `Queue.Enqueue` | Validates client data, serializes sequence assignment under the queue lock, generates server metadata, journals the complete message, and only then commits it to the correct heap. | `EnqueueInput, time.Time` → `Message, error` | Called by `Manager.Enqueue`; calls validation, ID generation, journal, `heap.Push` | 40 | **Critical** |
| `Queue.Dequeue` | Locks queue state, promotes newly eligible messages, selects the ready root, journals its removal, and removes it only after durability succeeds. | `time.Time` → `Message, bool, error` | Called by `Manager.Dequeue`; calls promotion, journal, `heap.Pop` | 16 | **Critical** |
| `Queue.promoteEligible` | Moves every delayed root whose deadline has arrived into the ready heap so normal priority/FIFO/LIFO ranking can decide consumption. | `time.Time` → no return | Called by `Queue.Dequeue`; calls delayed `heap.Pop` and ready `heap.Push` | 6 | High |
| `validateEnqueueInput` | Enforces the core body, UTF-8, size, and delay contract independently of HTTP so all callers receive the same validation behavior. | `EnqueueInput` → `error` | Called by `Queue.Enqueue`; calls UTF-8 validation | 15 | High |
| `generateMessageID` | Reads cryptographic randomness and formats it as a UUID-v4-style identifier, ensuring accepted messages receive server-owned IDs. | none → `string, error` | Called by `Queue.Enqueue` through `idGenerator`; calls crypto/rand and hex encoding | 12 | Supporting |

### [`internal/storage/wal.go`](../../internal/storage/wal.go)

Purpose: Implements the append-only JSON-lines durability layer, including synchronization, serialized access, replay, partial-tail repair, and write rollback.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `Open` | Creates the private WAL directory if needed and opens or creates the file for both replay and future appends. | filesystem path → `*WAL, error` | Called by server `run` and tests; calls filesystem APIs | 11 | High |
| `WAL.Append` | Validates and JSON-encodes one record, serializes access with the WAL mutex, appends the full newline-terminated entry, syncs it, and rolls back an incomplete/unsynced write. | `Record` → `error` | Called by manager creation and `QueueJournal`; calls validation, `writeAll`, `rollback` | 29 | **Critical** |
| `WAL.Replay` | Reads complete records from the start in append order, validates each record, removes an interrupted partial final line, and restores the file position for future appends. | WAL receiver → `[]Record, error` | Called by `NewManager`; calls JSON decoding, validation, truncate/seek | 41 | **Critical** |
| `WAL.Close` | Serializes shutdown, makes repeated closes harmless, marks the WAL unavailable, then syncs and closes the underlying file while preserving all errors. | WAL receiver → `error` | Called by server cleanup and tests; calls `Sync`, `Close` | 9 | High |
| `WAL.rollback` | Repairs a failed append by truncating to its original offset, returning to file end, and syncing the repaired file while joining repair errors with the original cause. | original offset, cause → `error` | Called by `WAL.Append`; calls truncate, seek, sync | 6 | **Critical** |
| `writeAll` | Repeats writes until every encoded byte is accepted, treating a zero-byte write as `io.ErrShortWrite` so a WAL entry cannot be silently truncated. | `io.Writer, []byte` → `error` | Called by `WAL.Append`; calls `Writer.Write` | 13 | High |

### [`internal/storage/recovery.go`](../../internal/storage/recovery.go)

Purpose: Reduces an ordered WAL history into one current `queue.State` per named queue for runtime restoration.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `Recover` | Replays create/enqueue/dequeue semantics into temporary per-queue message maps, preserves each queue's highest sequence, rejects impossible history, sorts survivors deterministically, and emits restoration states. | `[]Record` → `map[string]queue.State, error` | Called by `NewManager`; calls record validation, map mutation, sorting | 51 | **Critical** |

### [`cmd/client/main.go`](../../cmd/client/main.go)

Purpose: Implements the demonstration command-line application, translating commands and flags into reusable HTTP-client calls and supporting continuous dequeue polling.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `main` | Creates a signal-cancelled context, runs the CLI with process arguments and standard streams, and converts a returned error into process output and exit status. | process environment → process exit | Program entry; calls `run` | 9 | High |
| `run` | Selects the requested subcommand, constructs the HTTP client from configuration, validates top-level command shape, and delegates to the matching command handler. | context, argument slice, output streams → `error` | Called by `main`; calls client constructor and `run*` functions | 34 | High |
| `runCreate` | Parses and validates the queue name and required FIFO/LIFO flag, calls the client create endpoint, and prints returned queue metadata. | context, client, args, streams → `error` | Called by `run`; calls `Client.CreateQueue`, `printJSON` | 19 | Supporting |
| `runGet` | Requires exactly one queue name, retrieves its metadata, and prints the result as formatted JSON. | context, client, args, output → `error` | Called by `run`; calls `Client.GetQueue`, `printJSON` | 10 | Supporting |
| `runEnqueue` | Parses body, priority, and delay flags; enforces CLI numeric bounds; sends the enqueue request; and prints the server-assigned message. | context, client, args, streams → `error` | Called by `run`; calls `Client.Enqueue`, `printJSON` | 32 | High |
| `runDequeue` | Requests one message, prints a friendly empty-queue message for `204`, or prints the consumed message as JSON. | context, client, args, output → `error` | Called by `run`; calls `Client.Dequeue`, `printJSON` | 14 | Supporting |
| `runWorker` | Continuously dequeues and prints available messages, waits for a configured interval when empty, and exits cleanly when its context is cancelled. | context, client, args, streams → `error` | Called by `run`; calls `Client.Dequeue`, timer, `printJSON` | 38 | High |
| `ignoreHelp` | Converts the flag package's help sentinel into success while preserving real parsing failures. | `error` → `error` | Called by flag-parsing command handlers; calls `errors.Is` | 6 | Trivial |
| `printJSON` | Produces consistently indented JSON for CLI command results. | writer, value → `error` | Called by command handlers; calls `json.Encoder` | 5 | Trivial |
| `printUsage` | Writes the static command reference to the selected output stream. | writer → no return | Called by `run`; calls `fmt.Fprintln` | 11 | Trivial |
| `environment` | Reads a CLI environment variable and supplies its default when unset. | name, fallback → `string` | Called by `run`; calls `os.Getenv` | 6 | Trivial |

## Tier 3: Queue-to-WAL Adapter

### [`internal/storage/journal.go`](../../internal/storage/journal.go)

Purpose: Adapts the queue engine's name-free `Journal` calls into named WAL records for one specific queue.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `NewQueueJournal` | Validates the shared WAL and queue name, then binds them so later queue mutation callbacks automatically write records for the correct named queue. | `*WAL, string` → `*QueueJournal, error` | Called by `NewManager`, `Manager.CreateQueue`; constructs adapter | 9 | High |
| `QueueJournal.RecordEnqueue` | Adds this journal's queue name to a complete message, creates an enqueue record, and delegates its durable append to the shared WAL. | `queue.Message` → `error` | Called by `Queue.Enqueue` through `Journal`; calls record constructor, `WAL.Append` | 3 | **Critical** |
| `QueueJournal.RecordDequeue` | Adds this journal's queue name to a message ID, creates a dequeue record, and delegates its durable append to the shared WAL. | message ID → `error` | Called by `Queue.Dequeue` through `Journal`; calls record constructor, `WAL.Append` | 3 | **Critical** |

## Tier 4: Queue and Storage Coordination

### [`internal/service/manager.go`](../../internal/service/manager.go)

Purpose: Owns the registry of named queues, coordinates durable queue creation, restores every queue during startup, and provides the application-facing queue operations.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `NewManager` | Replays the shared WAL, reduces records into queue states, constructs a name-bound journal for each state, restores each live queue, and returns the completed registry. | `*WAL, time.Time` → `*Manager, error` | Called by server `run`; calls `Replay`, `Recover`, `NewQueueJournal`, `Restore` | 31 | **Critical** |
| `Manager.CreateQueue` | Validates the external name and ordering, prepares the journal and empty queue, exclusively checks the registry, durably records creation, and only then adds the queue to memory. | name, `Ordering` → `QueueInfo, error` | Called by HTTP `createQueue`; calls journal/queue constructors and `WAL.Append` | 25 | **Critical** |
| `Manager.Queue` | Resolves a named queue and returns the public metadata required by the GET endpoint without exposing the mutable queue pointer. | name → `QueueInfo, error` | Called by HTTP `getQueue`; calls `getQueue`, `Queue.Ordering` | 7 | Supporting |
| `Manager.Enqueue` | Resolves the named queue and delegates the complete mutation to that queue, preserving its returned message and error. | name, `EnqueueInput`, time → `Message, error` | Called by HTTP `enqueue`; calls `getQueue`, `Queue.Enqueue` | 7 | High |
| `Manager.Dequeue` | Resolves the named queue and delegates consumption, preserving the message/empty/error distinction from the core engine. | name, time → `Message, bool, error` | Called by HTTP `dequeue`; calls `getQueue`, `Queue.Dequeue` | 7 | High |
| `Manager.getQueue` | Protects the registry with a read lock, returns the queue pointer for a name, and translates a missing map entry into `ErrQueueNotFound`. | name → `*Queue, error` | Called by manager query/enqueue/dequeue; reads queue map | 9 | High |

## Tier 5: HTTP Application Layer

### [`internal/httpapi/handler.go`](../../internal/httpapi/handler.go)

Purpose: Defines the server's HTTP contract, converts JSON/path data into service calls, maps domain errors to status codes, and serializes all responses.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `NewHandler` | Constructs the handler, installs the clock dependency, registers every method/path route, and stores the resulting mux behind the `http.Handler` interface. | `*service.Manager` → `*Handler` | Called by server `run`; registers endpoint methods | 11 | High |
| `Handler.ServeHTTP` | Passes each incoming request to the configured method-aware mux so it reaches the matching endpoint function. | response writer, request → HTTP response | Called by `net/http`; calls router `ServeHTTP` | 3 | Trivial |
| `Handler.createQueue` | Decodes create input, invokes durable manager creation, maps domain failures, and emits `201` with public queue metadata. | HTTP request → `201` or error response | Called by router; calls `decodeJSON`, `Manager.CreateQueue`, writers | 14 | High |
| `Handler.getQueue` | Extracts the path queue name, retrieves public queue metadata, and emits either `200` or a mapped service error. | HTTP request → `200` or error response | Called by router; calls `Manager.Queue`, response writers | 8 | Supporting |
| `Handler.enqueue` | Decodes the API enqueue model, validates safe delay conversion, converts seconds to `time.Duration`, calls the manager with server time, and emits the accepted message. | HTTP request → `201` or error response | Called by router; calls decoding, `Manager.Enqueue`, writers | 22 | **Critical** |
| `Handler.dequeue` | Calls the manager with path name and server time, converts the empty boolean into `204`, and returns a consumed message with `200`. | HTTP request → `200`, `204`, or error | Called by router; calls `Manager.Dequeue`, writers | 12 | High |
| `Handler.health` | Returns a constant health payload without touching queue or storage state. | HTTP request → `200 {status: ok}` | Called by router; calls `writeJSON` | 3 | Trivial |
| `decodeJSON` | Bounds request size, rejects unknown fields and malformed JSON, decodes into the endpoint-specific destination, and ensures exactly one JSON object exists. | writer, request, destination pointer → `error` | Called by create/enqueue; calls JSON decoder | 12 | High |
| `writeServiceError` | Centralizes translation from recognizable queue/service errors to stable HTTP status/code pairs while hiding unexpected internal failures behind a generic `500`. | writer, `error` → HTTP error response | Called by endpoint handlers; calls `writeError` | 18 | High |
| `writeError` | Wraps an API error code and message in the server's standard error envelope before serialization. | writer, status, code, message → HTTP response | Called by decoding and service error paths; calls `writeJSON` | 3 | Trivial |
| `writeJSON` | Applies the JSON content type and status code, then encodes any success or error model to the response stream. | writer, status, value → HTTP response | Called by every endpoint/error writer; calls JSON encoder | 5 | Supporting |

## Tier 6: Server Entry Point

### [`cmd/server/main.go`](../../cmd/server/main.go)

Purpose: Composes storage, recovery, service, and HTTP layers into the running process and owns graceful shutdown.

| Function | Detailed purpose | Input → output | Called by / calls | Size | Importance |
| --- | --- | --- | --- | ---: | --- |
| `main` | Runs server composition and converts any startup, serving, or shutdown error into a fatal process exit. | process environment → process exit | Program entry; calls `run`, `log.Fatal` | 5 | High |
| `run` | Opens the WAL, guarantees its close, restores the manager, constructs the HTTP server and timeouts, starts serving concurrently, then coordinates server failure or signal-driven graceful shutdown. | environment/process signals → `error` | Called by `main`; calls storage, manager, handler, HTTP server APIs | 47 | **Critical** |
| `environment` | Reads server configuration such as `WAL_PATH` and `PORT`, returning the provided default when a variable is unset. | name, fallback → `string` | Called by `run`; calls `os.Getenv` | 6 | Trivial |

## Recommended Deep-Dive Order

Start with the functions that define correctness, then inspect their supporting mechanics:

1. `Queue.Enqueue`
2. `Queue.Dequeue`
3. `WAL.Append`
4. `WAL.Replay`
5. `Recover`
6. `queue.Restore`
7. `Manager.NewManager`
8. `Manager.CreateQueue`
9. `readyHeap.Less` and `delayedHeap.Less`
10. `Client.request` if reviewing the demonstration application

Use the [detailed traces](traces.md) to see these functions cooperate. Once a function is selected, its deeper guide should add concrete input/output values, a key-parts breakdown, failure paths, and a line-by-line walkthrough without repeating the full project trace.
