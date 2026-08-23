package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

func TestQueueSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.wal")
	wal := openTestWAL(t, path)
	if err := wal.Append(NewCreateQueueRecord("emails", queue.FIFO)); err != nil {
		t.Fatalf("record queue creation: %v", err)
	}
	journal, err := NewQueueJournal(wal, "emails")
	if err != nil {
		t.Fatalf("NewQueueJournal() returned error: %v", err)
	}
	q, err := queue.NewWithJournal(queue.FIFO, journal)
	if err != nil {
		t.Fatalf("NewWithJournal() returned error: %v", err)
	}

	removed, err := q.Enqueue(queue.EnqueueInput{Body: "removed"}, storageTestTime)
	if err != nil {
		t.Fatalf("enqueue removed message: %v", err)
	}
	kept, err := q.Enqueue(queue.EnqueueInput{
		Body:  "delayed",
		Delay: time.Minute,
	}, storageTestTime)
	if err != nil {
		t.Fatalf("enqueue delayed message: %v", err)
	}
	message, ok, err := q.Dequeue(storageTestTime)
	if err != nil || !ok || message.ID != removed.ID {
		t.Fatalf("Dequeue() = (%q, %t, %v), want removed message", message.ID, ok, err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	reopened := openTestWAL(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	records, err := reopened.Replay()
	if err != nil {
		t.Fatalf("Replay() returned error: %v", err)
	}
	states, err := Recover(records)
	if err != nil {
		t.Fatalf("Recover() returned error: %v", err)
	}
	state := states["emails"]
	if state.Ordering != queue.FIFO || state.Sequence != 2 {
		t.Fatalf("recovered state = %+v, want FIFO at sequence 2", state)
	}
	if len(state.Messages) != 1 || state.Messages[0].ID != kept.ID {
		t.Fatalf("recovered messages = %+v, want only delayed message", state.Messages)
	}

	restoredJournal, err := NewQueueJournal(reopened, "emails")
	if err != nil {
		t.Fatalf("NewQueueJournal() after restart returned error: %v", err)
	}
	restored, err := queue.Restore(state, storageTestTime, restoredJournal)
	if err != nil {
		t.Fatalf("Restore() returned error: %v", err)
	}
	if _, ok, err := restored.Dequeue(kept.AvailableAt.Add(-time.Nanosecond)); err != nil || ok {
		t.Fatalf("Dequeue() before delay = (ok=%t, err=%v), want empty", ok, err)
	}
	message, ok, err = restored.Dequeue(kept.AvailableAt)
	if err != nil || !ok || message.ID != kept.ID {
		t.Fatalf("Dequeue() after delay = (%q, %t, %v), want delayed message", message.ID, ok, err)
	}
}
