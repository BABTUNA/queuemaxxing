package storage

import (
	"errors"
	"fmt"
	"sort"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

var ErrInvalidReplay = errors.New("invalid WAL replay")

type recoveredQueue struct {
	ordering queue.Ordering
	sequence uint64
	messages map[string]queue.Message
}

func Recover(records []Record) (map[string]queue.State, error) {
	queues := make(map[string]*recoveredQueue)
	for _, record := range records {
		if err := record.validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidReplay, err)
		}
		switch record.Operation {
		case CreateQueue:
			if _, exists := queues[record.Queue]; exists {
				return nil, fmt.Errorf("%w: queue %q already exists", ErrInvalidReplay, record.Queue)
			}
			queues[record.Queue] = &recoveredQueue{
				ordering: record.Ordering,
				messages: make(map[string]queue.Message),
			}
		case Enqueue:
			q, exists := queues[record.Queue]
			if !exists {
				return nil, fmt.Errorf("%w: queue %q does not exist", ErrInvalidReplay, record.Queue)
			}
			message := *record.Message
			q.messages[message.ID] = message
			if message.Sequence > q.sequence {
				q.sequence = message.Sequence
			}
		case Dequeue:
			q, exists := queues[record.Queue]
			if !exists {
				return nil, fmt.Errorf("%w: queue %q does not exist", ErrInvalidReplay, record.Queue)
			}
			delete(q.messages, record.MessageID)
		}
	}

	states := make(map[string]queue.State, len(queues))
	for name, recovered := range queues {
		messages := make([]queue.Message, 0, len(recovered.messages))
		for _, message := range recovered.messages {
			messages = append(messages, message)
		}
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].Sequence < messages[j].Sequence
		})
		states[name] = queue.State{
			Ordering: recovered.ordering,
			Sequence: recovered.sequence,
			Messages: messages,
		}
	}
	return states, nil
}
