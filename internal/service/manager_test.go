package service

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
	"github.com/BABTUNA/queuemaxxing/internal/storage"
)

var managerTestTime = time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)

func TestManagerCreatesAndFindsQueues(t *testing.T) {
	manager, wal := newTestManager(t, filepath.Join(t.TempDir(), "queue.wal"))
	t.Cleanup(func() { _ = wal.Close() })

	info, err := manager.CreateQueue("emails", queue.FIFO)
	if err != nil {
		t.Fatalf("CreateQueue() returned error: %v", err)
	}
	if info.Name != "emails" || info.Ordering != queue.FIFO {
		t.Fatalf("CreateQueue() = %+v, want emails FIFO", info)
	}
	if _, err := manager.CreateQueue("emails", queue.FIFO); !errors.Is(err, ErrQueueAlreadyExists) {
		t.Fatalf("duplicate CreateQueue() error = %v", err)
	}
	if _, err := manager.CreateQueue("Bad Name", queue.FIFO); !errors.Is(err, ErrInvalidQueueName) {
		t.Fatalf("invalid CreateQueue() error = %v", err)
	}
	if _, err := manager.Queue("missing"); !errors.Is(err, ErrQueueNotFound) {
		t.Fatalf("Queue(missing) error = %v", err)
	}
}

func TestManagerRecoversFromWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.wal")
	manager, wal := newTestManager(t, path)
	if _, err := manager.CreateQueue("emails", queue.LIFO); err != nil {
		t.Fatalf("CreateQueue() returned error: %v", err)
	}
	want, err := manager.Enqueue("emails", queue.EnqueueInput{Body: "recover me"}, managerTestTime)
	if err != nil {
		t.Fatalf("Enqueue() returned error: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	reopened, err := storage.Open(path)
	if err != nil {
		t.Fatalf("storage.Open() returned error: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	recovered, err := NewManager(reopened, managerTestTime)
	if err != nil {
		t.Fatalf("NewManager() after restart returned error: %v", err)
	}
	info, err := recovered.Queue("emails")
	if err != nil || info.Ordering != queue.LIFO {
		t.Fatalf("Queue() = (%+v, %v), want LIFO", info, err)
	}
	got, ok, err := recovered.Dequeue("emails", managerTestTime)
	if err != nil || !ok || got.ID != want.ID {
		t.Fatalf("Dequeue() = (%q, %t, %v), want recovered message", got.ID, ok, err)
	}
}

func TestManagerConcurrentCreateAllowsOneQueue(t *testing.T) {
	manager, wal := newTestManager(t, filepath.Join(t.TempDir(), "queue.wal"))
	t.Cleanup(func() { _ = wal.Close() })

	var successes atomic.Int32
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, err := manager.CreateQueue("emails", queue.FIFO); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrQueueAlreadyExists) {
				t.Errorf("CreateQueue() returned unexpected error: %v", err)
			}
		}()
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful creates = %d, want 1", successes.Load())
	}
}

func newTestManager(t *testing.T, path string) (*Manager, *storage.WAL) {
	t.Helper()
	wal, err := storage.Open(path)
	if err != nil {
		t.Fatalf("storage.Open() returned error: %v", err)
	}
	manager, err := NewManager(wal, managerTestTime)
	if err != nil {
		_ = wal.Close()
		t.Fatalf("NewManager() returned error: %v", err)
	}
	return manager, wal
}
