package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/httpapi"
	"github.com/BABTUNA/queuemaxxing/internal/queue"
	"github.com/BABTUNA/queuemaxxing/internal/service"
	"github.com/BABTUNA/queuemaxxing/internal/storage"
)

func TestClientQueueWorkflow(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Health(ctx); err != nil {
		t.Fatalf("Health() returned error: %v", err)
	}
	created, err := client.CreateQueue(ctx, "emails", queue.FIFO)
	if err != nil || created.Name != "emails" || created.Ordering != queue.FIFO {
		t.Fatalf("CreateQueue() = (%+v, %v), want emails FIFO", created, err)
	}
	got, err := client.GetQueue(ctx, "emails")
	if err != nil || got != created {
		t.Fatalf("GetQueue() = (%+v, %v), want %+v", got, err, created)
	}

	enqueued, err := client.Enqueue(ctx, "emails", EnqueueInput{Body: "Send email", Priority: 5})
	if err != nil {
		t.Fatalf("Enqueue() returned error: %v", err)
	}
	dequeued, ok, err := client.Dequeue(ctx, "emails")
	if err != nil || !ok || dequeued.ID != enqueued.ID {
		t.Fatalf("Dequeue() = (%q, %t, %v), want enqueued message", dequeued.ID, ok, err)
	}
	if _, ok, err := client.Dequeue(ctx, "emails"); err != nil || ok {
		t.Fatalf("empty Dequeue() = (ok=%t, err=%v), want empty", ok, err)
	}
}

func TestClientDecodesAPIErrors(t *testing.T) {
	client := newTestClient(t)
	_, err := client.GetQueue(context.Background(), "missing")
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("GetQueue() error = %v, want APIError", err)
	}
	if apiError.StatusCode != http.StatusNotFound || apiError.Code != "queue_not_found" {
		t.Fatalf("APIError = %+v, want 404 queue_not_found", apiError)
	}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	wal, err := storage.Open(filepath.Join(t.TempDir(), "queue.wal"))
	if err != nil {
		t.Fatalf("storage.Open() returned error: %v", err)
	}
	t.Cleanup(func() { _ = wal.Close() })
	manager, err := service.NewManager(wal, time.Now())
	if err != nil {
		t.Fatalf("service.NewManager() returned error: %v", err)
	}
	handler := httpapi.NewHandler(manager)
	client := New("http://queue.test")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Result(), nil
	})}
	return client
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
