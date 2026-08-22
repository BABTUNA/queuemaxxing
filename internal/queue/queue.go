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
}

// New creates an empty queue with the requested equal-priority ordering mode.
func New(ordering Ordering) (*Queue, error) {
	if ordering != FIFO && ordering != LIFO {
		return nil, ErrInvalidOrdering
	}

	return newQueue(ordering, generateMessageID), nil
}

func newQueue(ordering Ordering, generator idGenerator) *Queue {
	return &Queue{
		ordering: ordering,
		ready: readyHeap{
			ordering: ordering,
		},
		generateID: generator,
	}
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

	q.sequence++
	createdAt := now.UTC()
	message := Message{
		ID:          id,
		Body:        input.Body,
		Priority:    input.Priority,
		Sequence:    q.sequence,
		CreatedAt:   createdAt,
		AvailableAt: createdAt.Add(input.Delay),
	}

	if input.Delay == 0 {
		heap.Push(&q.ready, message)
	} else {
		heap.Push(&q.delayed, message)
	}

	return message, nil
}

// Dequeue removes and returns the highest-ranked message eligible at now.
func (q *Queue) Dequeue(now time.Time) (Message, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.promoteEligible(now.UTC())
	if q.ready.Len() == 0 {
		return Message{}, false
	}

	return heap.Pop(&q.ready).(Message), true
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
