package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/queue"
)

const maxResponseBytes = 2 * 1024 * 1024

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type QueueInfo struct {
	Name     string         `json:"name"`
	Ordering queue.Ordering `json:"ordering"`
}

type EnqueueInput struct {
	Body         string `json:"body"`
	Priority     int32  `json:"priority"`
	DelaySeconds int64  `json:"delay_seconds"`
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
	}
	return e.Code + ": " + e.Message
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) CreateQueue(ctx context.Context, name string, ordering queue.Ordering) (QueueInfo, error) {
	var info QueueInfo
	err := c.do(ctx, http.MethodPost, "/queues", map[string]any{
		"name":     name,
		"ordering": ordering,
	}, &info)
	return info, err
}

func (c *Client) GetQueue(ctx context.Context, name string) (QueueInfo, error) {
	var info QueueInfo
	err := c.do(ctx, http.MethodGet, "/queues/"+url.PathEscape(name), nil, &info)
	return info, err
}

func (c *Client) Enqueue(ctx context.Context, name string, input EnqueueInput) (queue.Message, error) {
	var message queue.Message
	err := c.do(ctx, http.MethodPost, "/queues/"+url.PathEscape(name)+"/messages", input, &message)
	return message, err
}

func (c *Client) Dequeue(ctx context.Context, name string) (queue.Message, bool, error) {
	var message queue.Message
	status, err := c.request(ctx, http.MethodPost, "/queues/"+url.PathEscape(name)+"/dequeue", nil, &message)
	if err != nil {
		return queue.Message{}, false, err
	}
	if status == http.StatusNoContent {
		return queue.Message{}, false, nil
	}
	return message, true, nil
}

func (c *Client) Health(ctx context.Context) error {
	var response struct {
		Status string `json:"status"`
	}
	if err := c.do(ctx, http.MethodGet, "/health", nil, &response); err != nil {
		return err
	}
	if response.Status != "ok" {
		return fmt.Errorf("unexpected health status %q", response.Status)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body, destination any) error {
	_, err := c.request(ctx, method, path, body, destination)
	return err
}

func (c *Client) request(ctx context.Context, method, path string, body, destination any) (int, error) {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("encode request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	limitedBody := io.LimitReader(response.Body, maxResponseBytes)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, decodeAPIError(response.StatusCode, limitedBody)
	}
	if response.StatusCode == http.StatusNoContent || destination == nil {
		return response.StatusCode, nil
	}
	if err := json.NewDecoder(limitedBody).Decode(destination); err != nil {
		return response.StatusCode, fmt.Errorf("decode response: %w", err)
	}
	return response.StatusCode, nil
}

func decodeAPIError(status int, body io.Reader) error {
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&response); err != nil || response.Error.Message == "" {
		return &APIError{StatusCode: status, Message: http.StatusText(status)}
	}
	return &APIError{
		StatusCode: status,
		Code:       response.Error.Code,
		Message:    response.Error.Message,
	}
}
