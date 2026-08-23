package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
	"github.com/BABTUNA/queuemaxxing/internal/service"
	"github.com/BABTUNA/queuemaxxing/internal/storage"
)

var handlerTestTime = time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)

func TestHTTPQueueWorkflow(t *testing.T) {
	handler := newTestHandler(t)

	response := request(t, handler, http.MethodPost, "/queues", `{"name":"emails","ordering":"fifo"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", response.Code, response.Body.String())
	}

	response = request(t, handler, http.MethodGet, "/queues/emails", "")
	if response.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", response.Code)
	}
	var info service.QueueInfo
	decodeResponse(t, response, &info)
	if info.Name != "emails" || info.Ordering != queue.FIFO {
		t.Fatalf("queue info = %+v, want emails FIFO", info)
	}

	response = request(t, handler, http.MethodPost, "/queues/emails/messages", `{"body":"Send email","priority":5}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("enqueue status = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	var enqueued queue.Message
	decodeResponse(t, response, &enqueued)

	response = request(t, handler, http.MethodPost, "/queues/emails/dequeue", "")
	if response.Code != http.StatusOK {
		t.Fatalf("dequeue status = %d, want 200", response.Code)
	}
	var dequeued queue.Message
	decodeResponse(t, response, &dequeued)
	if dequeued.ID != enqueued.ID {
		t.Fatalf("dequeued ID = %q, want %q", dequeued.ID, enqueued.ID)
	}

	response = request(t, handler, http.MethodPost, "/queues/emails/dequeue", "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("empty dequeue status = %d, want 204", response.Code)
	}
}

func TestHTTPErrors(t *testing.T) {
	handler := newTestHandler(t)
	request(t, handler, http.MethodPost, "/queues", `{"name":"emails","ordering":"fifo"}`)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{name: "malformed JSON", method: http.MethodPost, path: "/queues", body: `{`, status: 400, code: "invalid_request"},
		{name: "duplicate queue", method: http.MethodPost, path: "/queues", body: `{"name":"emails","ordering":"fifo"}`, status: 409, code: "queue_already_exists"},
		{name: "invalid ordering", method: http.MethodPost, path: "/queues", body: `{"name":"jobs","ordering":"random"}`, status: 400, code: "invalid_ordering"},
		{name: "missing queue", method: http.MethodGet, path: "/queues/missing", status: 404, code: "queue_not_found"},
		{name: "invalid delay", method: http.MethodPost, path: "/queues/emails/messages", body: `{"body":"A","delay_seconds":901}`, status: 400, code: "invalid_delay"},
		{name: "empty body", method: http.MethodPost, path: "/queues/emails/messages", body: `{}`, status: 400, code: "invalid_body"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(t, handler, test.method, test.path, test.body)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
			var body errorResponse
			decodeResponse(t, response, &body)
			if body.Error.Code != test.code {
				t.Fatalf("error code = %q, want %q", body.Error.Code, test.code)
			}
		})
	}
}

func TestHealth(t *testing.T) {
	handler := newTestHandler(t)
	response := request(t, handler, http.MethodGet, "/health", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("health response = %d %s", response.Code, response.Body.String())
	}
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	wal, err := storage.Open(filepath.Join(t.TempDir(), "queue.wal"))
	if err != nil {
		t.Fatalf("storage.Open() returned error: %v", err)
	}
	t.Cleanup(func() { _ = wal.Close() })
	manager, err := service.NewManager(wal, handlerTestTime)
	if err != nil {
		t.Fatalf("service.NewManager() returned error: %v", err)
	}
	handler := NewHandler(manager)
	handler.now = func() time.Time { return handlerTestTime }
	return handler
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
