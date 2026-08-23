package storage

import (
	"errors"
	"fmt"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

type Operation string

const (
	CreateQueue Operation = "create_queue"
	Enqueue     Operation = "enqueue"
	Dequeue     Operation = "dequeue"
)

var ErrInvalidRecord = errors.New("invalid WAL record")

type Record struct {
	Operation Operation      `json:"operation"`
	Queue     string         `json:"queue"`
	Ordering  queue.Ordering `json:"ordering,omitempty"`
	Message   *queue.Message `json:"message,omitempty"`
	MessageID string         `json:"message_id,omitempty"`
}

func NewCreateQueueRecord(name string, ordering queue.Ordering) Record {
	return Record{Operation: CreateQueue, Queue: name, Ordering: ordering}
}

func NewEnqueueRecord(name string, message queue.Message) Record {
	return Record{Operation: Enqueue, Queue: name, Message: &message}
}

func NewDequeueRecord(name, messageID string) Record {
	return Record{Operation: Dequeue, Queue: name, MessageID: messageID}
}

func (r Record) validate() error {
	if r.Queue == "" {
		return fmt.Errorf("%w: empty queue name", ErrInvalidRecord)
	}
	switch r.Operation {
	case CreateQueue:
		if r.Ordering != queue.FIFO && r.Ordering != queue.LIFO {
			return fmt.Errorf("%w: invalid ordering", ErrInvalidRecord)
		}
	case Enqueue:
		if r.Message == nil {
			return fmt.Errorf("%w: enqueue has no message", ErrInvalidRecord)
		}
	case Dequeue:
		if r.MessageID == "" {
			return fmt.Errorf("%w: dequeue has no message ID", ErrInvalidRecord)
		}
	default:
		return fmt.Errorf("%w: unknown operation", ErrInvalidRecord)
	}
	return nil
}
