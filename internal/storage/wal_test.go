package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

var storageTestTime = time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)

func TestWALAppendAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.wal")
	wal := openTestWAL(t, path)
	message := testMessage("A", 1)
	want := []Record{
		NewCreateQueueRecord("emails", queue.FIFO),
		NewEnqueueRecord("emails", message),
		NewDequeueRecord("emails", message.ID),
	}
	for _, record := range want {
		if err := wal.Append(record); err != nil {
			t.Fatalf("Append() returned error: %v", err)
		}
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	reopened := openTestWAL(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.Replay()
	if err != nil {
		t.Fatalf("Replay() returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Replay() = %+v, want %+v", got, want)
	}
}

func TestReplayIgnoresPartialFinalLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.wal")
	wal := openTestWAL(t, path)
	want := NewCreateQueueRecord("emails", queue.FIFO)
	if err := wal.Append(want); err != nil {
		t.Fatalf("Append() returned error: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open WAL tail: %v", err)
	}
	if _, err := file.WriteString(`{"operation":"enqueue"`); err != nil {
		_ = file.Close()
		t.Fatalf("write partial record: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close WAL tail: %v", err)
	}

	reopened := openTestWAL(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	records, err := reopened.Replay()
	if err != nil {
		t.Fatalf("Replay() returned error: %v", err)
	}
	if len(records) != 1 || !reflect.DeepEqual(records[0], want) {
		t.Fatalf("Replay() = %+v, want only create record", records)
	}
}

func openTestWAL(t *testing.T, path string) *WAL {
	t.Helper()
	wal, err := Open(path)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	return wal
}

func testMessage(id string, sequence uint64) queue.Message {
	return queue.Message{
		ID:          id,
		Body:        id,
		Sequence:    sequence,
		CreatedAt:   storageTestTime,
		AvailableAt: storageTestTime,
	}
}
