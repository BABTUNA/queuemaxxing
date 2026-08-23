package storage

import (
	"errors"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

var (
	ErrNilWAL         = errors.New("WAL must not be nil")
	ErrEmptyQueueName = errors.New("queue name must not be empty")
)

// QueueJournal adapts a WAL to the queue.Journal interface for one named
// queue.
type QueueJournal struct {
	wal   *WAL
	queue string
}

func NewQueueJournal(wal *WAL, name string) (*QueueJournal, error) {
	if wal == nil {
		return nil, ErrNilWAL
	}
	if name == "" {
		return nil, ErrEmptyQueueName
	}
	return &QueueJournal{wal: wal, queue: name}, nil
}

func (j *QueueJournal) RecordEnqueue(message queue.Message) error {
	return j.wal.Append(NewEnqueueRecord(j.queue, message))
}

func (j *QueueJournal) RecordDequeue(messageID string) error {
	return j.wal.Append(NewDequeueRecord(j.queue, messageID))
}

var _ queue.Journal = (*QueueJournal)(nil)
