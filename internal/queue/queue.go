package queue

import (
	"container/heap"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"sync"
	"time"
	"unicode/utf8"
)

type idGenerator func() (string, error)

// Queue is an in-memory queue ordered by priority and FIFO/LIFO sequence.
// All mutable state is protected by mu.
type Queue struct {
	mu         sync.Mutex
	ordering   Ordering
	sequence   uint64
	ready      readyHeap
	delayed    delayedHeap
	generateID idGenerator
	journal    Journal
}

// New creates an empty queue with the requested equal-priority ordering mode.
func New(ordering Ordering) (*Queue, error) {
	if ordering != FIFO && ordering != LIFO {
		return nil, ErrInvalidOrdering
	}

	return newQueue(ordering, generateMessageID, noopJournal{}), nil
}

// NewWithJournal creates a queue whose semantic mutations must be recorded by
// journal before they are committed to memory.
func NewWithJournal(ordering Ordering, journal Journal) (*Queue, error) {
	if ordering != FIFO && ordering != LIFO {
		return nil, ErrInvalidOrdering
	}
	if journal == nil {
		return nil, ErrNilJournal
	}

	return newQueue(ordering, generateMessageID, journal), nil
}

func newQueue(ordering Ordering, generator idGenerator, journal Journal) *Queue {
	return &Queue{
		ordering: ordering,
		ready: readyHeap{
			ordering: ordering,
		},
		generateID: generator,
		journal:    journal,
	}
}

// Restore reconstructs a queue without journaling the historical messages.
// Future mutations are recorded through journal.
func Restore(state State, now time.Time, journal Journal) (*Queue, error) {
	q, err := NewWithJournal(state.Ordering, journal)
	if err != nil {
		return nil, err
	}
	q.sequence = state.Sequence

	now = now.UTC()
	for _, message := range state.Messages {
		if message.AvailableAt.After(now) {
			heap.Push(&q.delayed, message)
		} else {
			heap.Push(&q.ready, message)
		}
	}

	return q, nil
}

// Ordering returns the queue's immutable equal-priority ordering mode.
func (q *Queue) Ordering() Ordering {
	return q.ordering
}

// Enqueue validates and inserts a message using now as the server-controlled
// creation time. A positive delay keeps the message ineligible until its
// AvailableAt timestamp.
func (q *Queue) Enqueue(input EnqueueInput, now time.Time) (Message, error) {
	if err := validateEnqueueInput(input); err != nil {
		return Message{}, err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.sequence == math.MaxUint64 {
		return Message{}, ErrSequenceExhausted
	}

	id, err := q.generateID()
	if err != nil {
		return Message{}, fmt.Errorf("generate message ID: %w", err)
	}

	nextSequence := q.sequence + 1
	createdAt := now.UTC()
	message := Message{
		ID:          id,
		Body:        input.Body,
		Priority:    input.Priority,
		Sequence:    nextSequence,
		CreatedAt:   createdAt,
		AvailableAt: createdAt.Add(input.Delay),
	}
	if err := q.journal.RecordEnqueue(message); err != nil {
		return Message{}, fmt.Errorf("record enqueue: %w", err)
	}

	q.sequence = nextSequence
	if input.Delay == 0 {
		heap.Push(&q.ready, message)
	} else {
		heap.Push(&q.delayed, message)
	}

	return message, nil
}

// Dequeue removes and returns the highest-ranked message eligible at now.
func (q *Queue) Dequeue(now time.Time) (Message, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.promoteEligible(now.UTC())
	if q.ready.Len() == 0 {
		return Message{}, false, nil
	}

	message := q.ready.items[0]
	if err := q.journal.RecordDequeue(message.ID); err != nil {
		return Message{}, false, fmt.Errorf("record dequeue: %w", err)
	}

	return heap.Pop(&q.ready).(Message), true, nil
}

func (q *Queue) promoteEligible(now time.Time) {
	for q.delayed.Len() > 0 && !q.delayed[0].AvailableAt.After(now) {
		message := heap.Pop(&q.delayed).(Message)
		heap.Push(&q.ready, message)
	}
}

func validateEnqueueInput(input EnqueueInput) error {
	if len(input.Body) == 0 {
		return ErrEmptyBody
	}
	if !utf8.ValidString(input.Body) {
		return ErrInvalidUTF8
	}
	if len(input.Body) > MaxBodyBytes {
		return ErrBodyTooLarge
	}
	if input.Delay < 0 || input.Delay > MaxDelay {
		return ErrInvalidDelay
	}
	return nil
}

func generateMessageID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
