package queue

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

var testTime = time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)

func TestNewValidatesOrdering(t *testing.T) {
	t.Parallel()

	for _, ordering := range []Ordering{FIFO, LIFO} {
		ordering := ordering
		t.Run(string(ordering), func(t *testing.T) {
			t.Parallel()

			q, err := New(ordering)
			if err != nil {
				t.Fatalf("New(%q) returned error: %v", ordering, err)
			}
			if got := q.Ordering(); got != ordering {
				t.Fatalf("Ordering() = %q, want %q", got, ordering)
			}
		})
	}

	if _, err := New(Ordering("random")); !errors.Is(err, ErrInvalidOrdering) {
		t.Fatalf("New(random) error = %v, want ErrInvalidOrdering", err)
	}
}

func TestQueueOrdering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ordering Ordering
		inputs   []EnqueueInput
		want     []string
	}{
		{
			name:     "FIFO",
			ordering: FIFO,
			inputs:   messages("A", "B", "C"),
			want:     []string{"A", "B", "C"},
		},
		{
			name:     "LIFO",
			ordering: LIFO,
			inputs:   messages("A", "B", "C"),
			want:     []string{"C", "B", "A"},
		},
		{
			name:     "priority with FIFO tie-breaker",
			ordering: FIFO,
			inputs: []EnqueueInput{
				{Body: "A", Priority: 5},
				{Body: "B", Priority: 10},
				{Body: "C", Priority: 5},
			},
			want: []string{"B", "A", "C"},
		},
		{
			name:     "priority with LIFO tie-breaker",
			ordering: LIFO,
			inputs: []EnqueueInput{
				{Body: "A", Priority: 5},
				{Body: "B", Priority: 10},
				{Body: "C", Priority: 5},
			},
			want: []string{"B", "C", "A"},
		},
		{
			name:     "negative priorities",
			ordering: FIFO,
			inputs: []EnqueueInput{
				{Body: "lowest", Priority: -10},
				{Body: "default"},
				{Body: "lower", Priority: -1},
			},
			want: []string{"default", "lower", "lowest"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			q := newTestQueue(tt.ordering)
			for _, input := range tt.inputs {
				if _, err := q.Enqueue(input, testTime); err != nil {
					t.Fatalf("Enqueue(%q) returned error: %v", input.Body, err)
				}
			}

			if got := drainBodies(q, testTime); !equalStrings(got, tt.want) {
				t.Fatalf("dequeue order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnqueueAssignsMessageMetadata(t *testing.T) {
	t.Parallel()

	q := newTestQueue(FIFO)
	input := EnqueueInput{Body: "hello", Delay: 30 * time.Second}

	message, err := q.Enqueue(input, testTime.In(time.FixedZone("offset", -7*60*60)))
	if err != nil {
		t.Fatalf("Enqueue() returned error: %v", err)
	}

	if message.ID != "test-id-1" {
		t.Fatalf("ID = %q, want test-id-1", message.ID)
	}
	if message.Priority != 0 {
		t.Fatalf("Priority = %d, want default 0", message.Priority)
	}
	if message.Sequence != 1 {
		t.Fatalf("Sequence = %d, want 1", message.Sequence)
	}
	if message.CreatedAt.Location() != time.UTC || !message.CreatedAt.Equal(testTime) {
		t.Fatalf("CreatedAt = %v, want %v in UTC", message.CreatedAt, testTime)
	}
	if want := testTime.Add(30 * time.Second); !message.AvailableAt.Equal(want) {
		t.Fatalf("AvailableAt = %v, want %v", message.AvailableAt, want)
	}
}

func TestDelayedMessageEligibilityAndPriority(t *testing.T) {
	t.Parallel()

	q := newTestQueue(FIFO)
	delayed, err := q.Enqueue(EnqueueInput{
		Body:     "delayed-high",
		Priority: 100,
		Delay:    10 * time.Second,
	}, testTime)
	if err != nil {
		t.Fatalf("enqueue delayed message: %v", err)
	}
	if _, err := q.Enqueue(EnqueueInput{Body: "ready-low", Priority: 1}, testTime); err != nil {
		t.Fatalf("enqueue ready message: %v", err)
	}

	message, ok := q.Dequeue(testTime)
	if !ok || message.Body != "ready-low" {
		t.Fatalf("Dequeue(at enqueue) = (%q, %t), want ready-low", message.Body, ok)
	}

	if message, ok := q.Dequeue(delayed.AvailableAt.Add(-time.Nanosecond)); ok {
		t.Fatalf("Dequeue(before AvailableAt) returned %q, want empty", message.Body)
	}

	message, ok = q.Dequeue(delayed.AvailableAt)
	if !ok || message.Body != "delayed-high" {
		t.Fatalf("Dequeue(at AvailableAt) = (%q, %t), want delayed-high", message.Body, ok)
	}
}

func TestDelayedMessagesRetainSequenceTieBreaker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ordering Ordering
		want     []string
	}{
		{ordering: FIFO, want: []string{"A", "C"}},
		{ordering: LIFO, want: []string{"C", "A"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.ordering), func(t *testing.T) {
			t.Parallel()

			q := newTestQueue(tt.ordering)
			for _, body := range []string{"A", "C"} {
				_, err := q.Enqueue(EnqueueInput{
					Body:     body,
					Priority: 10,
					Delay:    10 * time.Second,
				}, testTime)
				if err != nil {
					t.Fatalf("Enqueue(%q) returned error: %v", body, err)
				}
			}
			if _, err := q.Enqueue(EnqueueInput{Body: "B", Priority: 1}, testTime); err != nil {
				t.Fatalf("Enqueue(B) returned error: %v", err)
			}

			message, ok := q.Dequeue(testTime)
			if !ok || message.Body != "B" {
				t.Fatalf("first Dequeue() = (%q, %t), want B", message.Body, ok)
			}

			if got := drainBodies(q, testTime.Add(10*time.Second)); !equalStrings(got, tt.want) {
				t.Fatalf("delayed dequeue order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnqueueValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input EnqueueInput
		want  error
	}{
		{name: "empty body", input: EnqueueInput{}, want: ErrEmptyBody},
		{name: "invalid UTF-8", input: EnqueueInput{Body: string([]byte{0xff})}, want: ErrInvalidUTF8},
		{name: "oversized body", input: EnqueueInput{Body: strings.Repeat("a", MaxBodyBytes+1)}, want: ErrBodyTooLarge},
		{name: "negative delay", input: EnqueueInput{Body: "A", Delay: -time.Second}, want: ErrInvalidDelay},
		{name: "excessive delay", input: EnqueueInput{Body: "A", Delay: MaxDelay + time.Nanosecond}, want: ErrInvalidDelay},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			q := newTestQueue(FIFO)
			if _, err := q.Enqueue(tt.input, testTime); !errors.Is(err, tt.want) {
				t.Fatalf("Enqueue() error = %v, want %v", err, tt.want)
			}
			if q.sequence != 0 || q.ready.Len() != 0 || q.delayed.Len() != 0 {
				t.Fatal("invalid enqueue mutated queue state")
			}
		})
	}
}

func TestEnqueueAcceptsContractLimits(t *testing.T) {
	t.Parallel()

	q := newTestQueue(FIFO)
	message, err := q.Enqueue(EnqueueInput{
		Body:  strings.Repeat("a", MaxBodyBytes),
		Delay: MaxDelay,
	}, testTime)
	if err != nil {
		t.Fatalf("Enqueue(at limits) returned error: %v", err)
	}
	if want := testTime.Add(MaxDelay); !message.AvailableAt.Equal(want) {
		t.Fatalf("AvailableAt = %v, want %v", message.AvailableAt, want)
	}
}

func TestSequenceExhaustionDoesNotMutateQueue(t *testing.T) {
	t.Parallel()

	q := newTestQueue(FIFO)
	q.sequence = math.MaxUint64

	if _, err := q.Enqueue(EnqueueInput{Body: "A"}, testTime); !errors.Is(err, ErrSequenceExhausted) {
		t.Fatalf("Enqueue() error = %v, want ErrSequenceExhausted", err)
	}
	if q.ready.Len() != 0 || q.delayed.Len() != 0 {
		t.Fatal("sequence exhaustion mutated queue state")
	}
}

func TestIDGenerationFailureDoesNotMutateQueue(t *testing.T) {
	t.Parallel()

	want := errors.New("random source failed")
	q := newQueue(FIFO, func() (string, error) {
		return "", want
	})

	if _, err := q.Enqueue(EnqueueInput{Body: "A"}, testTime); !errors.Is(err, want) {
		t.Fatalf("Enqueue() error = %v, want wrapped generator error", err)
	}
	if q.sequence != 0 || q.ready.Len() != 0 || q.delayed.Len() != 0 {
		t.Fatal("ID generation failure mutated queue state")
	}
}

func TestGenerateMessageIDReturnsUUIDv4(t *testing.T) {
	t.Parallel()

	id, err := generateMessageID()
	if err != nil {
		t.Fatalf("generateMessageID() returned error: %v", err)
	}

	compact := strings.ReplaceAll(id, "-", "")
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != 16 {
		t.Fatalf("generateMessageID() = %q, want a 16-byte UUID: %v", id, err)
	}
	if version := decoded[6] >> 4; version != 4 {
		t.Fatalf("UUID version = %d, want 4", version)
	}
	if variant := decoded[8] >> 6; variant != 2 {
		t.Fatalf("UUID variant = %d, want RFC 4122 variant 2", variant)
	}
}

func TestConcurrentEnqueueAssignsUniqueIDsAndSequences(t *testing.T) {
	q := newTestQueue(FIFO)
	const total = 256

	results := make(chan Message, total)
	errorsCh := make(chan error, total)
	var workers sync.WaitGroup
	workers.Add(total)

	for i := 0; i < total; i++ {
		i := i
		go func() {
			defer workers.Done()
			message, err := q.Enqueue(EnqueueInput{Body: fmt.Sprintf("message-%d", i)}, testTime)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- message
		}()
	}

	workers.Wait()
	close(results)
	close(errorsCh)

	for err := range errorsCh {
		t.Fatalf("concurrent Enqueue() returned error: %v", err)
	}

	ids := make(map[string]struct{}, total)
	sequences := make(map[uint64]struct{}, total)
	for message := range results {
		if _, exists := ids[message.ID]; exists {
			t.Fatalf("duplicate message ID %q", message.ID)
		}
		ids[message.ID] = struct{}{}
		if _, exists := sequences[message.Sequence]; exists {
			t.Fatalf("duplicate sequence %d", message.Sequence)
		}
		sequences[message.Sequence] = struct{}{}
	}

	if len(ids) != total || len(sequences) != total {
		t.Fatalf("received %d IDs and %d sequences, want %d each", len(ids), len(sequences), total)
	}
	for sequence := uint64(1); sequence <= total; sequence++ {
		if _, exists := sequences[sequence]; !exists {
			t.Fatalf("missing sequence %d", sequence)
		}
	}
}

func TestConcurrentDequeueReturnsEachMessageOnce(t *testing.T) {
	q := newTestQueue(FIFO)
	const total = 256

	for i := 0; i < total; i++ {
		if _, err := q.Enqueue(EnqueueInput{Body: fmt.Sprintf("message-%d", i)}, testTime); err != nil {
			t.Fatalf("Enqueue() returned error: %v", err)
		}
	}

	start := make(chan struct{})
	results := make(chan Message, total)
	var workers sync.WaitGroup
	workers.Add(total * 2)
	for i := 0; i < total*2; i++ {
		go func() {
			defer workers.Done()
			<-start
			if message, ok := q.Dequeue(testTime); ok {
				results <- message
			}
		}()
	}

	close(start)
	workers.Wait()
	close(results)

	seen := make(map[string]struct{}, total)
	for message := range results {
		if _, exists := seen[message.ID]; exists {
			t.Fatalf("message %q was dequeued more than once", message.ID)
		}
		seen[message.ID] = struct{}{}
	}
	if len(seen) != total {
		t.Fatalf("dequeued %d unique messages, want %d", len(seen), total)
	}
	if message, ok := q.Dequeue(testTime); ok {
		t.Fatalf("queue not empty after concurrent dequeue; got %q", message.Body)
	}
}

func TestMixedConcurrentOperationsDoNotLoseMessages(t *testing.T) {
	q := newTestQueue(FIFO)
	const total = 256

	dequeued := make(chan Message, total)
	var workers sync.WaitGroup
	workers.Add(total * 2)

	for i := 0; i < total; i++ {
		i := i
		go func() {
			defer workers.Done()
			if _, err := q.Enqueue(EnqueueInput{Body: fmt.Sprintf("message-%d", i)}, testTime); err != nil {
				t.Errorf("Enqueue() returned error: %v", err)
			}
		}()
		go func() {
			defer workers.Done()
			if message, ok := q.Dequeue(testTime); ok {
				dequeued <- message
			}
		}()
	}

	workers.Wait()
	for {
		message, ok := q.Dequeue(testTime)
		if !ok {
			break
		}
		dequeued <- message
	}
	close(dequeued)

	seen := make(map[string]struct{}, total)
	for message := range dequeued {
		if _, exists := seen[message.ID]; exists {
			t.Fatalf("message %q was dequeued more than once", message.ID)
		}
		seen[message.ID] = struct{}{}
	}
	if len(seen) != total {
		t.Fatalf("observed %d unique messages, want %d", len(seen), total)
	}
}

func messages(bodies ...string) []EnqueueInput {
	inputs := make([]EnqueueInput, 0, len(bodies))
	for _, body := range bodies {
		inputs = append(inputs, EnqueueInput{Body: body})
	}
	return inputs
}

func newTestQueue(ordering Ordering) *Queue {
	var nextID int
	return newQueue(ordering, func() (string, error) {
		nextID++
		return fmt.Sprintf("test-id-%d", nextID), nil
	})
}

func drainBodies(q *Queue, now time.Time) []string {
	var bodies []string
	for {
		message, ok := q.Dequeue(now)
		if !ok {
			return bodies
		}
		bodies = append(bodies, message.Body)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
