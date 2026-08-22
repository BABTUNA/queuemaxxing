package queue

import "time"

const (
	// MaxBodyBytes is the largest UTF-8 encoded message body accepted by the
	// core queue.
	MaxBodyBytes = 256 * 1024

	// MaxDelay is the largest delivery delay accepted by the core queue.
	MaxDelay = 15 * time.Minute
)

// Ordering controls how messages with equal priority are dequeued.
type Ordering string

const (
	FIFO Ordering = "fifo"
	LIFO Ordering = "lifo"
)

// Message is an accepted queue message with server-assigned metadata.
type Message struct {
	ID          string    `json:"id"`
	Body        string    `json:"body"`
	Priority    int32     `json:"priority"`
	Sequence    uint64    `json:"sequence"`
	CreatedAt   time.Time `json:"created_at"`
	AvailableAt time.Time `json:"available_at"`
}

// EnqueueInput contains the client-controlled fields for a new message.
type EnqueueInput struct {
	Body     string
	Priority int32
	Delay    time.Duration
}
