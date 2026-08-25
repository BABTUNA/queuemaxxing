# `internal/queue/errors.go`

[Back to Code Guide](../code-guide.md) · [Source File](../../internal/queue/errors.go)

## Why This File Exists

This file defines the stable errors produced by the queue package. Central error values allow higher layers to recognize a failure without comparing error-message strings.

## Dependencies

Project dependencies: None.

Standard-library dependency:

```go
import "errors"
```

## Files That Depend on It

| File | How it uses the errors |
| --- | --- |
| `queue.go` ([source](../../internal/queue/queue.go)) | Returns validation, journal, and sequence errors |
| `httpapi/handler.go` ([source](../../internal/httpapi/handler.go)) | Maps recognized queue errors to HTTP responses |
| `queue_test.go` ([source](../../internal/queue/queue_test.go)) | Asserts the expected failure with `errors.Is` |

## Defined Errors

```go
var (
    ErrInvalidOrdering   = errors.New("ordering must be fifo or lifo")
    ErrEmptyBody         = errors.New("message body must not be empty")
    ErrInvalidUTF8       = errors.New("message body must be valid UTF-8")
    ErrBodyTooLarge      = errors.New("message body exceeds 256 KiB")
    ErrInvalidDelay      = errors.New("delay must be between 0 and 15 minutes")
    ErrNilJournal        = errors.New("journal must not be nil")
    ErrSequenceExhausted = errors.New("message sequence exhausted")
)
```

These are often called sentinel errors: shared error values that callers can identify with `errors.Is`.

## Error Reference

| Error | Trigger | Initially returned by |
| --- | --- | --- |
| `ErrInvalidOrdering` | Ordering is not `fifo` or `lifo` | `New` or `NewWithJournal` |
| `ErrEmptyBody` | Body is empty | `validateEnqueueInput` |
| `ErrInvalidUTF8` | Body contains invalid UTF-8 | `validateEnqueueInput` |
| `ErrBodyTooLarge` | Body exceeds 256 KiB | `validateEnqueueInput` |
| `ErrInvalidDelay` | Delay is negative or over 15 minutes | `validateEnqueueInput` |
| `ErrNilJournal` | Durable queue was given no journal | `NewWithJournal` |
| `ErrSequenceExhausted` | Sequence reached `math.MaxUint64` | `Queue.Enqueue` |

## Why Shared Error Values Matter

Suppose `Queue.Enqueue` returns:

```go
ErrEmptyBody
```

The HTTP layer checks:

```go
if errors.Is(err, queue.ErrEmptyBody) {
    // return HTTP 400
}
```

This is safer than:

```go
if err.Error() == "message body must not be empty" {
```

The text can be changed later without breaking the logic that recognizes the error.

## Walkthrough: Invalid Delay

Client request:

```json
{
  "body": "Send later",
  "delay_seconds": 901
}
```

The HTTP handler rejects this value before constructing a `time.Duration` and returns:

```json
{
  "error": {
    "code": "invalid_delay",
    "message": "delay_seconds must be between 0 and 900"
  }
}
```

The queue package also protects itself when called without HTTP:

```go
_, err := q.Enqueue(queue.EnqueueInput{
    Body:  "Send later",
    Delay: 16 * time.Minute,
}, time.Now())
```

Result:

```go
err == queue.ErrInvalidDelay
```

Complete path for a direct queue call:

```text
Queue.Enqueue
→ validateEnqueueInput
→ ErrInvalidDelay
→ caller
```

Complete HTTP path for a queue-originated validation error:

```text
Queue.Enqueue
→ ErrInvalidDelay
→ Manager.Enqueue
→ Handler.enqueue
→ writeServiceError
→ HTTP 400 invalid_delay
```

## Walkthrough: Wrapped Errors

Some functions add context around an underlying error:

```go
return Message{}, fmt.Errorf("record enqueue: %w", err)
```

The `%w` keeps the original error inside the new error. That means this still works:

```go
if errors.Is(err, originalError) {
    // true even though the error was wrapped
}
```

Example shape:

```text
Original error:
WAL is closed

Wrapped error returned by Queue.Enqueue:
record enqueue: WAL is closed
```

The caller gets useful context while retaining the original error identity.

## Walkthrough: Sequence Exhaustion

Before enqueue:

```go
q.sequence = math.MaxUint64
```

`Queue.Enqueue` checks the counter before adding one:

```go
if q.sequence == math.MaxUint64 {
    return Message{}, ErrSequenceExhausted
}
```

Output:

```go
queue.Message{}, queue.ErrSequenceExhausted
```

No message is journaled and no heap is changed. This prevents the sequence from wrapping back to zero.

## Inputs and Outputs

`errors.go` contains values rather than functions, so it has no direct function inputs or outputs. The values become outputs of functions in `queue.go`.

Example:

```go
q, err := queue.New(queue.Ordering("random"))
```

Output:

```go
q   = nil
err = queue.ErrInvalidOrdering
```

Another example:

```go
message, err := q.Enqueue(queue.EnqueueInput{}, time.Now())
```

Output:

```go
message = queue.Message{}
err     = queue.ErrEmptyBody
```

## What to Remember

If asked about this file, the short answer is:

> `errors.go` defines stable queue errors. The queue engine returns them, tests identify them with `errors.Is`, and the HTTP handler converts recognized errors into client-facing status codes. It is a Tier 0 file because it only depends on the standard library.
