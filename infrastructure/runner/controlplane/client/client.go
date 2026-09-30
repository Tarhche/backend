// Package client reaches the runner control plane over HTTP.
//
// Everything it asks for is an answer: what is streamed rather than answered —
// a task's output as it is written, what becomes of one, a command running
// inside one — arrives another way, as the runner's own messages or through the
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
	"strconv"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	runnerControlPlane "github.com/khanzadimahdi/testproject/domain/runner/controlplane"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// controlPlaneStack is the client's own name for what the contract calls a Stack,
// so the wire mapping can build one without importing its own package.
type controlPlaneStack = runnerControlPlane.Stack

// requestTimeout bounds a call to the control plane.
const requestTimeout = 15 * time.Second

// Client is the runner control plane, reached over its HTTP API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

var _ runnerControlPlane.Client = &Client{}

// New builds a client for the control plane at baseURL, e.g. "http://runner-controlplane:80".
// New builds a client for the control plane. It answers about tasks; reaching
// one is the ingress's business and no longer passes through here.
func New(baseURL string) (*Client, error) {
	parsed, err := usable(baseURL, "runner control plane")
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

func (c *Client) Tasks(ctx context.Context, ownerUUID string, page uint) (runnerControlPlane.Page[task.Task], error) {
	var payload tasksPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/tasks", listing(ownerUUID, page)), nil, &payload); err != nil {
		return runnerControlPlane.Page[task.Task]{}, err
	}

	items := make([]task.Task, len(payload.Items))
	for i := range payload.Items {
		items[i] = payload.Items[i].toTask()
	}

	return runnerControlPlane.Page[task.Task]{
		Items:       items,
		TotalPages:  payload.Pagination.TotalPages,
		CurrentPage: payload.Pagination.CurrentPage,
	}, nil
}

func (c *Client) Task(ctx context.Context, uuid string) (task.Task, error) {
	var payload taskPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/tasks/"+url.PathEscape(uuid), nil), nil, &payload); err != nil {
		return task.Task{}, err
	}

	return payload.toTask(), nil
}

func (c *Client) TaskOf(ctx context.Context, ownerUUID string, uuid string) (task.Task, error) {
	query := url.Values{}
	query.Set("owner", ownerUUID)

	var payload taskPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/tasks/"+url.PathEscape(uuid), query), nil, &payload); err != nil {
		return task.Task{}, err
	}

	return payload.toTask(), nil
}

func (c *Client) RunTask(ctx context.Context, spec runnerControlPlane.TaskSpec, ownerUUID string) (task.Task, error) {
	body := map[string]any{"name": spec.Name, "owner_uuid": ownerUUID, "service": spec.Service}

	var payload taskPayload
	if err := c.call(ctx, http.MethodPost, c.path("/api/tasks/run", nil), body, &payload); err != nil {
		return task.Task{}, err
	}

	return payload.toTask(), nil
}

func (c *Client) StopTask(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/tasks/"+url.PathEscape(uuid)+"/stop", nil), nil, nil)
}

func (c *Client) KillTask(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/tasks/"+url.PathEscape(uuid)+"/kill", nil), nil, nil)
}

func (c *Client) RestartTask(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/tasks/"+url.PathEscape(uuid)+"/restart", nil), nil, nil)
}

// DeleteTask removes a task whether or not it is still running: the
// dashboard's delete is a request to have it gone.
func (c *Client) DeleteTask(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path("/api/tasks/"+url.PathEscape(uuid), url.Values{"force": {"true"}}), nil, nil)
}

func (c *Client) TaskLogs(ctx context.Context, uuid string, after time.Time, limit uint) ([]task.Log, error) {
	query := url.Values{}
	if !after.IsZero() {
		query.Set("after", after.UTC().Format(time.RFC3339Nano))
	}
	if limit > 0 {
		query.Set("limit", strconv.FormatUint(uint64(limit), 10))
	}

	var payload logsPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/tasks/"+url.PathEscape(uuid)+"/logs", query), nil, &payload); err != nil {
		return nil, err
	}

	logs := make([]task.Log, len(payload.Items))
	for i := range payload.Items {
		logs[i] = payload.Items[i].toLog(uuid)
	}

	return logs, nil
}

func (c *Client) Stacks(ctx context.Context, ownerUUID string, page uint) (runnerControlPlane.Page[runnerControlPlane.Stack], error) {
	var payload stacksPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/stacks", listing(ownerUUID, page)), nil, &payload); err != nil {
		return runnerControlPlane.Page[runnerControlPlane.Stack]{}, err
	}

	items := make([]runnerControlPlane.Stack, len(payload.Items))
	for i := range payload.Items {
		items[i] = payload.Items[i].toStack()
	}

	return runnerControlPlane.Page[runnerControlPlane.Stack]{
		Items:       items,
		TotalPages:  payload.Pagination.TotalPages,
		CurrentPage: payload.Pagination.CurrentPage,
	}, nil
}

func (c *Client) Stack(ctx context.Context, uuid string) (runnerControlPlane.Stack, error) {
	var payload stackPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/stacks/"+url.PathEscape(uuid), nil), nil, &payload); err != nil {
		return runnerControlPlane.Stack{}, err
	}

	return payload.toStack(), nil
}

func (c *Client) StackOf(ctx context.Context, ownerUUID string, uuid string) (runnerControlPlane.Stack, error) {
	query := url.Values{}
	query.Set("owner", ownerUUID)

	var payload stackPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/stacks/"+url.PathEscape(uuid), query), nil, &payload); err != nil {
		return runnerControlPlane.Stack{}, err
	}

	return payload.toStack(), nil
}

func (c *Client) RunStack(ctx context.Context, spec runnerControlPlane.StackSpec, ownerUUID string) (runnerControlPlane.Stack, error) {
	body := map[string]any{"name": spec.Name, "owner_uuid": ownerUUID, "services": spec.Services}

	var payload stackPayload
	if err := c.call(ctx, http.MethodPost, c.path("/api/stacks/run", nil), body, &payload); err != nil {
		return runnerControlPlane.Stack{}, err
	}

	return payload.toStack(), nil
}

func (c *Client) StopStack(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/stacks/"+url.PathEscape(uuid)+"/stop", nil), nil, nil)
}

func (c *Client) KillStack(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/stacks/"+url.PathEscape(uuid)+"/kill", nil), nil, nil)
}

func (c *Client) RestartStack(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/stacks/"+url.PathEscape(uuid)+"/restart", nil), nil, nil)
}

func (c *Client) DeleteStack(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path("/api/stacks/"+url.PathEscape(uuid), nil), nil, nil)
}

// ValidationError carries what the control plane refused, so the dashboard can show
// the caller which field it was rather than a bare failure.
type ValidationError struct {
	ValidationErrors domain.ValidationErrors
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("the runner refused the request: %v", e.ValidationErrors)
}

// listing is what a page of somebody's tasks or stacks is asked for by.
func listing(ownerUUID string, page uint) url.Values {
	query := url.Values{"page": {strconv.FormatUint(uint64(page), 10)}}

	if len(ownerUUID) > 0 {
		query.Set("owner", ownerUUID)
	}

	return query
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
			return fmt.Errorf("the runner refused the request")
		}

		return &ValidationError{ValidationErrors: refusal.Errors}

	case response.StatusCode >= http.StatusBadRequest:
		return fmt.Errorf("the runner answered %s", response.Status)
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
