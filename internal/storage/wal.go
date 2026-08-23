package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrClosed     = errors.New("WAL is closed")
	ErrInvalidWAL = errors.New("invalid WAL")
)

// WAL is an append-only JSON-lines file. Successful appends are synced before
// returning.
type WAL struct {
	mu     sync.Mutex
	file   *os.File
	closed bool
}

func Open(path string) (*WAL, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create WAL directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open WAL: %w", err)
	}
	return &WAL{file: file}, nil
}

func (w *WAL) Append(record Record) error {
	if err := record.validate(); err != nil {
		return err
	}

	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode WAL record: %w", err)
	}
	line = append(line, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}

	start, err := w.file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek WAL end: %w", err)
	}
	if err := writeAll(w.file, line); err != nil {
		return w.rollback(start, fmt.Errorf("write WAL record: %w", err))
	}
	if err := w.file.Sync(); err != nil {
		return w.rollback(start, fmt.Errorf("sync WAL: %w", err))
	}
	return nil
}

// Replay returns complete records in order. A partial final line is ignored
// and removed because it represents an interrupted append.
func (w *WAL) Replay() ([]Record, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, ErrClosed
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek WAL start: %w", err)
	}

	reader := bufio.NewReader(w.file)
	var records []Record
	var validBytes int64
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			if len(line) > 0 {
				if truncateErr := w.file.Truncate(validBytes); truncateErr != nil {
					return nil, fmt.Errorf("truncate partial WAL record: %w", truncateErr)
				}
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read WAL: %w", err)
		}

		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidWAL, err)
		}
		if err := record.validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidWAL, err)
		}
		records = append(records, record)
		validBytes += int64(len(line))
	}

	_, err := w.file.Seek(0, io.SeekEnd)
	return records, err
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return errors.Join(w.file.Sync(), w.file.Close())
}

func (w *WAL) rollback(offset int64, cause error) error {
	truncateErr := w.file.Truncate(offset)
	_, seekErr := w.file.Seek(0, io.SeekEnd)
	syncErr := w.file.Sync()
	return errors.Join(cause, truncateErr, seekErr, syncErr)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
