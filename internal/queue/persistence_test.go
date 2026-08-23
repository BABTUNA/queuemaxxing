package queue

import (
	"errors"
	"testing"
)

type journalStub struct {
	enqueueErr error
	dequeueErr error
}

func (j *journalStub) RecordEnqueue(Message) error {
	return j.enqueueErr
}

func (j *journalStub) RecordDequeue(string) error {
	return j.dequeueErr
}

func TestJournalFailureDoesNotMutateQueue(t *testing.T) {
	want := errors.New("disk write failed")
	journal := &journalStub{enqueueErr: want}
	q, err := NewWithJournal(FIFO, journal)
	if err != nil {
		t.Fatalf("NewWithJournal() returned error: %v", err)
	}

	if _, err := q.Enqueue(EnqueueInput{Body: "A"}, testTime); !errors.Is(err, want) {
		t.Fatalf("Enqueue() error = %v, want disk error", err)
	}
	if q.sequence != 0 || q.ready.Len() != 0 {
		t.Fatal("failed enqueue mutated the queue")
	}

	journal.enqueueErr = nil
	message, err := q.Enqueue(EnqueueInput{Body: "A"}, testTime)
	if err != nil {
		t.Fatalf("Enqueue() returned error: %v", err)
	}
	journal.dequeueErr = want
	if _, ok, err := q.Dequeue(testTime); !errors.Is(err, want) || ok {
		t.Fatalf("Dequeue() = (ok=%t, err=%v), want disk error", ok, err)
	}
	if q.ready.Len() != 1 || q.ready.items[0].ID != message.ID {
		t.Fatal("failed dequeue removed the message")
	}
}
