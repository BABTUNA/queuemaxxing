package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

func TestConcurrentHTTPDequeueReturnsEachMessageOnce(t *testing.T) {
	const total = 100
	handler := newTestHandler(t)
	if response := request(t, handler, http.MethodPost, "/queues", `{"name":"jobs","ordering":"fifo"}`); response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", response.Code)
	}
	for index := range total {
		body := fmt.Sprintf(`{"body":"job-%d"}`, index)
		if response := request(t, handler, http.MethodPost, "/queues/jobs/messages", body); response.Code != http.StatusCreated {
			t.Fatalf("enqueue %d status = %d, want 201", index, response.Code)
		}
	}

	start := make(chan struct{})
	results := make(chan queue.Message, total)
	errors := make(chan error, total)
	var workers sync.WaitGroup
	for range total {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			response := request(t, handler, http.MethodPost, "/queues/jobs/dequeue", "")
			if response.Code != http.StatusOK {
				errors <- fmt.Errorf("dequeue status = %d, want 200", response.Code)
				return
			}
			var message queue.Message
			if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
				errors <- fmt.Errorf("decode dequeue response: %w", err)
				return
			}
			results <- message
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errors)

	for err := range errors {
		t.Error(err)
	}
	seen := make(map[string]struct{}, total)
	for message := range results {
		if _, exists := seen[message.ID]; exists {
			t.Errorf("message %q dequeued more than once", message.ID)
		}
		seen[message.ID] = struct{}{}
	}
	if len(seen) != total {
		t.Fatalf("unique dequeues = %d, want %d", len(seen), total)
	}
	if response := request(t, handler, http.MethodPost, "/queues/jobs/dequeue", ""); response.Code != http.StatusNoContent {
		t.Fatalf("final dequeue status = %d, want 204", response.Code)
	}
}
