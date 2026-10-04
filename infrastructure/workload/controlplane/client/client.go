// Package client reaches the workload control plane over HTTP.
//
// Everything it asks for is an answer: what is streamed rather than answered —
// a task's output as it is written, what becomes of one, a command running
// inside one — arrives another way, as the workload's own messages or through the
// ingress.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// requestTimeout bounds a call to the control plane.
const requestTimeout = 15 * time.Second

// Client is the workload control plane, reached over its HTTP API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

var _ workloadControlPlane.Client = &Client{}

// New builds a client for the control plane at baseURL, e.g.
// "http://workload-controlplane:80". It answers about tasks; reaching one is
// the ingress's business and does not pass through here.
func New(baseURL string) (*Client, error) {
	parsed, err := usable(baseURL, "workload control plane")
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:    parsed,
		httpClient: &http.Client{Timeout: requestTimeout},
	}, nil
}

func usable(raw string, what string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("the %s url is not usable: %w", what, err)
	}

	if len(parsed.Scheme) == 0 || len(parsed.Host) == 0 {
		return nil, fmt.Errorf("the %s url needs a scheme and a host, got %q", what, raw)
	}

	return parsed, nil
}

func (c *Client) Task(ctx context.Context, uuid string) (task.Task, error) {
	var payload taskPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/tasks/"+url.PathEscape(uuid), nil), nil, &payload); err != nil {
		return task.Task{}, err
	}

	return payload.toTask(), nil
}

// DeleteTask removes a task whether or not it is still running: a delete is
// a request to have it gone.
func (c *Client) DeleteTask(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path("/api/tasks/"+url.PathEscape(uuid), url.Values{"force": {"true"}}), nil, nil)
}

// ValidationError carries what the control plane refused, so the dashboard can show
// the caller which field it was rather than a bare failure.
type ValidationError struct {
	ValidationErrors domain.ValidationErrors
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("the workload refused the request: %v", e.ValidationErrors)
}

func (c *Client) path(path string, query url.Values) string {
	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + path

	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	return u.String()
}

// call makes one request and decodes its answer. A 404 becomes
// domain.ErrNotExists and a 400 becomes a ValidationError, so the layers above
// deal in the errors they already know.
func (c *Client) call(ctx context.Context, method string, endpoint string, body any, out any) error {
	var payload io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}

		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	switch {
	case response.StatusCode == http.StatusNotFound:
		return domain.ErrNotExists

	case response.StatusCode == http.StatusBadRequest:
		var refusal struct {
			Errors domain.ValidationErrors `json:"errors"`
		}

		if err := json.NewDecoder(response.Body).Decode(&refusal); err != nil {
			return fmt.Errorf("the workload refused the request")
		}

		return &ValidationError{ValidationErrors: refusal.Errors}

	case response.StatusCode >= http.StatusBadRequest:
		return fmt.Errorf("the workload answered %s", response.Status)
	}

	if out == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, response.Body)

		return nil
	}

	if err := json.NewDecoder(response.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	return nil
}
