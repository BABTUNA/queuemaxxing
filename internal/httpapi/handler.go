package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
	"github.com/BABTUNA/queuemaxxing/internal/service"
)

const maxRequestBytes = queue.MaxBodyBytes*6 + 1024

type Handler struct {
	manager *service.Manager
	now     func() time.Time
	router  http.Handler
}

func NewHandler(manager *service.Manager) *Handler {
	handler := &Handler{manager: manager, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.health)
	mux.HandleFunc("POST /queues", handler.createQueue)
	mux.HandleFunc("GET /queues/{name}", handler.getQueue)
	mux.HandleFunc("POST /queues/{name}/messages", handler.enqueue)
	mux.HandleFunc("POST /queues/{name}/dequeue", handler.dequeue)
	handler.router = mux
	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

type createQueueRequest struct {
	Name     string         `json:"name"`
	Ordering queue.Ordering `json:"ordering"`
}

func (h *Handler) createQueue(w http.ResponseWriter, r *http.Request) {
	var request createQueueRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	info, err := h.manager.CreateQueue(request.Name, request.Ordering)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (h *Handler) getQueue(w http.ResponseWriter, r *http.Request) {
	info, err := h.manager.Queue(r.PathValue("name"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type enqueueRequest struct {
	Body         string `json:"body"`
	Priority     int32  `json:"priority"`
	DelaySeconds int64  `json:"delay_seconds"`
}

func (h *Handler) enqueue(w http.ResponseWriter, r *http.Request) {
	var request enqueueRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.DelaySeconds < 0 || request.DelaySeconds > int64(queue.MaxDelay/time.Second) {
		writeError(w, http.StatusBadRequest, "invalid_delay", "delay_seconds must be between 0 and 900")
		return
	}

	message, err := h.manager.Enqueue(r.PathValue("name"), queue.EnqueueInput{
		Body:     request.Body,
		Priority: request.Priority,
		Delay:    time.Duration(request.DelaySeconds) * time.Second,
	}, h.now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, message)
}

func (h *Handler) dequeue(w http.ResponseWriter, r *http.Request) {
	message, ok, err := h.manager.Dequeue(r.PathValue("name"), h.now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, message)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidQueueName):
		writeError(w, http.StatusBadRequest, "invalid_queue_name", err.Error())
	case errors.Is(err, queue.ErrInvalidOrdering):
		writeError(w, http.StatusBadRequest, "invalid_ordering", err.Error())
	case errors.Is(err, queue.ErrEmptyBody), errors.Is(err, queue.ErrInvalidUTF8), errors.Is(err, queue.ErrBodyTooLarge):
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
	case errors.Is(err, queue.ErrInvalidDelay):
		writeError(w, http.StatusBadRequest, "invalid_delay", err.Error())
	case errors.Is(err, service.ErrQueueAlreadyExists):
		writeError(w, http.StatusConflict, "queue_already_exists", err.Error())
	case errors.Is(err, service.ErrQueueNotFound):
		writeError(w, http.StatusNotFound, "queue_not_found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: apiError{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
