package service

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
	"github.com/BABTUNA/queuemaxxing/internal/storage"
)

var (
	ErrInvalidQueueName   = errors.New("queue name must match [a-z0-9][a-z0-9_-]{0,63}")
	ErrQueueAlreadyExists = errors.New("queue already exists")
	ErrQueueNotFound      = errors.New("queue not found")
	ErrNilWAL             = errors.New("WAL must not be nil")
)

var queueNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type QueueInfo struct {
	Name     string         `json:"name"`
	Ordering queue.Ordering `json:"ordering"`
}

// Manager owns the named queues and their shared WAL.
type Manager struct {
	mu     sync.RWMutex
	queues map[string]*queue.Queue
	wal    *storage.WAL
}

// NewManager replays the WAL and restores every queue.
func NewManager(wal *storage.WAL, now time.Time) (*Manager, error) {
	if wal == nil {
		return nil, ErrNilWAL
	}

	records, err := wal.Replay()
	if err != nil {
		return nil, fmt.Errorf("replay WAL: %w", err)
	}
	states, err := storage.Recover(records)
	if err != nil {
		return nil, fmt.Errorf("recover queues: %w", err)
	}

	manager := &Manager{
		queues: make(map[string]*queue.Queue, len(states)),
		wal:    wal,
	}
	for name, state := range states {
		journal, err := storage.NewQueueJournal(wal, name)
		if err != nil {
			return nil, fmt.Errorf("create journal for %q: %w", name, err)
		}
		restored, err := queue.Restore(state, now, journal)
		if err != nil {
			return nil, fmt.Errorf("restore queue %q: %w", name, err)
		}
		manager.queues[name] = restored
	}
	return manager, nil
}

func (m *Manager) CreateQueue(name string, ordering queue.Ordering) (QueueInfo, error) {
	if !queueNamePattern.MatchString(name) {
		return QueueInfo{}, ErrInvalidQueueName
	}

	journal, err := storage.NewQueueJournal(m.wal, name)
	if err != nil {
		return QueueInfo{}, err
	}
	created, err := queue.NewWithJournal(ordering, journal)
	if err != nil {
		return QueueInfo{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.queues[name]; exists {
		return QueueInfo{}, ErrQueueAlreadyExists
	}
	if err := m.wal.Append(storage.NewCreateQueueRecord(name, ordering)); err != nil {
		return QueueInfo{}, fmt.Errorf("record queue creation: %w", err)
	}
	m.queues[name] = created
	return QueueInfo{Name: name, Ordering: ordering}, nil
}

func (m *Manager) Queue(name string) (QueueInfo, error) {
	q, err := m.getQueue(name)
	if err != nil {
		return QueueInfo{}, err
	}
	return QueueInfo{Name: name, Ordering: q.Ordering()}, nil
}

func (m *Manager) Enqueue(name string, input queue.EnqueueInput, now time.Time) (queue.Message, error) {
	q, err := m.getQueue(name)
	if err != nil {
		return queue.Message{}, err
	}
	return q.Enqueue(input, now)
}

func (m *Manager) Dequeue(name string, now time.Time) (queue.Message, bool, error) {
	q, err := m.getQueue(name)
	if err != nil {
		return queue.Message{}, false, err
	}
	return q.Dequeue(now)
}

func (m *Manager) getQueue(name string) (*queue.Queue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, exists := m.queues[name]
	if !exists {
		return nil, ErrQueueNotFound
	}
	return q, nil
}
