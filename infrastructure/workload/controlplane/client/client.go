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
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

const (
	// requestTimeout bounds a call that reads or writes the control plane's
	// records and nothing else.
	requestTimeout = 15 * time.Second

	// nodeRequestTimeout bounds a call the control plane answers by asking a
	// node — a log, what a Docker VM holds — which it gives up on after its own
	// request timeout, 30 seconds unless it is configured otherwise.
	nodeRequestTimeout = 45 * time.Second

	// pullRequestTimeout bounds a call that may have to wait for a Docker VM
	// to come up and pull an image: creating a container, pulling an image.
	// Unless it is configured otherwise, the control plane gives a Docker VM
	// made for a container five minutes to come up, and then waits for its
	// node as long as the node may take, which is three minutes for dockerd
	// and ten for the pull; the blog waits a little longer than all of that,
	// so the control plane's answer is the one that comes back.
	pullRequestTimeout = 20 * time.Minute
)

// Client is the workload control plane, reached over its HTTP API.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

var _ workloadControlPlane.Client = &Client{}

// New builds a client for the control plane at baseURL, e.g.
// "http://workload-controlplane:80". It answers about tasks, VMs, snapshots,
// containers and stacks; reaching one is the ingress's business and does not
// pass through here.
func New(baseURL string) (*Client, error) {
	parsed, err := usable(baseURL, "workload control plane")
	if err != nil {
		return nil, err
	}

	// each call is bounded by its own timeout, since the slowest of them takes
	// minutes and the fastest should not.
	return &Client{
		baseURL:    parsed,
		httpClient: &http.Client{},
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

// ValidationError carries what the control plane refused, so the dashboard can show
// the caller which field it was rather than a bare failure.
//
// Each value is a code, which the blog puts into the words of whoever asked.
// A refusal that came from a node — a VM that is not running, one that is not
// a Docker VM, a dockerd that did not come up — is one too, under the vm
// field, and also unwraps to the node's own error, so errors.Is still reads it
// as the domain's error it stands for.
type ValidationError struct {
	ValidationErrors domain.ValidationErrors

	cause error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("the workload refused the request: %v", e.ValidationErrors)
}

// Unwrap is the node's error a refusal stands for, when it stands for one.
func (e *ValidationError) Unwrap() error {
	return e.cause
}

// Refused is what was refused, field by field, as codes.
func (e *ValidationError) Refused() domain.ValidationErrors {
	return e.ValidationErrors
}

// refusedByNode is what a node's refusal is to whoever called. A VM that cannot
// be asked is something the person asking can do something about, so it is a
// refusal under the vm field; anything else is the node's error as it said it,
// which errors.Is reads as the domain's own: a not_found is domain.ErrNotExists.
func refusedByNode(refused *noderequest.Error) error {
	switch refused.Code {
	case noderequest.CodeNotRunning, noderequest.CodeNotDocker, noderequest.CodeDockerUnavailable:
		return &ValidationError{
			ValidationErrors: domain.ValidationErrors{"vm": string(refused.Code)},
			cause:            refused,
		}
	default:
		return refused
	}
}

func (c *Client) path(path string, query url.Values) string {
	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + path

	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	return u.String()
}

// owned is the query of a call narrowed to ownerUUID's own, with what else it
// asks; anything empty is left out.
func owned(ownerUUID string, rest url.Values) url.Values {
	query := url.Values{}
	for key, values := range rest {
		for _, value := range values {
			if len(value) > 0 {
				query.Add(key, value)
			}
		}
	}

	if len(ownerUUID) > 0 {
		query.Set("owner", ownerUUID)
	}

	return query
}

// call makes one request about the control plane's records.
func (c *Client) call(ctx context.Context, method string, endpoint string, body any, out any) error {
	return c.callWithin(ctx, requestTimeout, method, endpoint, body, out)
}

// callWithin makes one request and decodes its answer, giving up after
// timeout. A 404 becomes domain.ErrNotExists, a 400 a ValidationError, and
// what a node refused the error it stands for, so the layers above deal in the
// errors they already know.
func (c *Client) callWithin(ctx context.Context, timeout time.Duration, method string, endpoint string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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

	if response.StatusCode >= http.StatusBadRequest {
		return refusal(response)
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

// refusal is the error an answer of 400 or more stands for.
func refusal(response *http.Response) error {
	answer, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))

	var refused struct {
		Errors domain.ValidationErrors `json:"errors"`
		Error  *noderequest.Error      `json:"error"`
	}

	decoded := json.Unmarshal(answer, &refused) == nil

	switch {
	case decoded && refused.Error != nil && len(refused.Error.Code) > 0:
		return refusedByNode(refused.Error)

	case response.StatusCode == http.StatusNotFound:
		return domain.ErrNotExists

	case response.StatusCode == http.StatusBadRequest:
		if !decoded {
			return fmt.Errorf("the workload refused the request")
		}

		return &ValidationError{ValidationErrors: refused.Errors}
	}

	return fmt.Errorf("the workload answered %s", response.Status)
}
