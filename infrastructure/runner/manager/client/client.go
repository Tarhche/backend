// Package client reaches the runner manager over HTTP, and over websockets for
// the two things that are streams rather than answers: a container's log as it
// is written, and a command running inside one.
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
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// managerStack is the client's own name for what the contract calls a Stack,
// so the wire mapping can build one without importing its own package.
type managerStack = runnerManager.Stack

// requestTimeout bounds a call to the manager. It does not apply to the
// streams, which are meant to stay open.
const requestTimeout = 15 * time.Second

// Client is the runner manager, reached over its HTTP API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

var _ runnerManager.Client = &Client{}

// New builds a client for the manager at baseURL, e.g. "http://runner-manager:80".
func New(baseURL string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSuffix(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("the runner manager url is not usable: %w", err)
	}

	if len(parsed.Scheme) == 0 || len(parsed.Host) == 0 {
		return nil, fmt.Errorf("the runner manager url needs a scheme and a host, got %q", baseURL)
	}

	return &Client{
		baseURL:    parsed,
		httpClient: &http.Client{Timeout: requestTimeout},
	}, nil
}

func (c *Client) Containers(ctx context.Context, page uint) (runnerManager.Page[task.Task], error) {
	var payload tasksPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/tasks", url.Values{"page": {strconv.FormatUint(uint64(page), 10)}}), nil, &payload); err != nil {
		return runnerManager.Page[task.Task]{}, err
	}

	items := make([]task.Task, len(payload.Items))
	for i := range payload.Items {
		items[i] = payload.Items[i].toTask()
	}

	return runnerManager.Page[task.Task]{
		Items:       items,
		TotalPages:  payload.Pagination.TotalPages,
		CurrentPage: payload.Pagination.CurrentPage,
	}, nil
}

func (c *Client) Container(ctx context.Context, uuid string) (task.Task, error) {
	var payload taskPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/tasks/"+url.PathEscape(uuid), nil), nil, &payload); err != nil {
		return task.Task{}, err
	}

	return payload.toTask(), nil
}

func (c *Client) RunContainer(ctx context.Context, spec runnerManager.ContainerSpec, ownerUUID string) (task.Task, error) {
	body := map[string]any{"name": spec.Name, "owner_uuid": ownerUUID, "service": spec.Service}

	var payload taskPayload
	if err := c.call(ctx, http.MethodPost, c.path("/api/containers/run", nil), body, &payload); err != nil {
		return task.Task{}, err
	}

	return payload.toTask(), nil
}

func (c *Client) StopContainer(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/tasks/"+url.PathEscape(uuid)+"/stop", nil), nil, nil)
}

func (c *Client) KillContainer(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/tasks/"+url.PathEscape(uuid)+"/kill", nil), nil, nil)
}

func (c *Client) RestartContainer(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path("/api/tasks/"+url.PathEscape(uuid)+"/restart", nil), nil, nil)
}

// DeleteContainer removes a container whether or not it is still running: the
// dashboard's delete is a request to have it gone.
func (c *Client) DeleteContainer(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path("/api/tasks/"+url.PathEscape(uuid), url.Values{"force": {"true"}}), nil, nil)
}

func (c *Client) ContainerLogs(ctx context.Context, uuid string, after time.Time, limit uint) ([]container.Log, error) {
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

	logs := make([]container.Log, len(payload.Items))
	for i := range payload.Items {
		logs[i] = payload.Items[i].toLog(uuid)
	}

	return logs, nil
}

func (c *Client) Stacks(ctx context.Context, page uint) (runnerManager.Page[runnerManager.Stack], error) {
	var payload stacksPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/stacks", url.Values{"page": {strconv.FormatUint(uint64(page), 10)}}), nil, &payload); err != nil {
		return runnerManager.Page[runnerManager.Stack]{}, err
	}

	items := make([]runnerManager.Stack, len(payload.Items))
	for i := range payload.Items {
		items[i] = payload.Items[i].toStack()
	}

	return runnerManager.Page[runnerManager.Stack]{
		Items:       items,
		TotalPages:  payload.Pagination.TotalPages,
		CurrentPage: payload.Pagination.CurrentPage,
	}, nil
}

func (c *Client) Stack(ctx context.Context, uuid string) (runnerManager.Stack, error) {
	var payload stackPayload
	if err := c.call(ctx, http.MethodGet, c.path("/api/stacks/"+url.PathEscape(uuid), nil), nil, &payload); err != nil {
		return runnerManager.Stack{}, err
	}

	return payload.toStack(), nil
}

func (c *Client) RunStack(ctx context.Context, spec runnerManager.StackSpec, ownerUUID string) (runnerManager.Stack, error) {
	body := map[string]any{"name": spec.Name, "owner_uuid": ownerUUID, "services": spec.Services}

	var payload stackPayload
	if err := c.call(ctx, http.MethodPost, c.path("/api/stacks/run", nil), body, &payload); err != nil {
		return runnerManager.Stack{}, err
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

// ValidationError carries what the manager refused, so the dashboard can show
// the caller which field it was rather than a bare failure.
type ValidationError struct {
	ValidationErrors domain.ValidationErrors
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("the runner refused the request: %v", e.ValidationErrors)
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
