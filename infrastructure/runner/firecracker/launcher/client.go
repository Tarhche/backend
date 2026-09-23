// Package launcher is an orchestrator's side of the launcher: what it asks of
// the host its machines run on, over the unix socket the launcher takes orders
// on.
package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

const (
	// maxAnswer bounds what is read back of an answer.
	maxAnswer = 4 << 20

	// requestTimeout bounds one order. Starting a machine is the slowest of
	// them, and it is quick or not happening.
	requestTimeout = 30 * time.Second

	// the address requests are made to. It names nothing: every connection
	// goes to the one socket this client was made for.
	launcherAddress = "http://launcher"
)

// Client asks the launcher for machines and networks.
type Client struct {
	http *http.Client
}

var _ machine.Launcher = &Client{}

// NewClient builds a client for the launcher taking orders at socketPath.
func NewClient(socketPath string) *Client {
	var dialer net.Dialer

	return &Client{
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

func (c *Client) Launch(ctx context.Context, spec machine.Spec) (machine.Machine, error) {
	var answer struct {
		Machine machine.Machine `json:"machine"`
	}

	return answer.Machine, c.do(ctx, http.MethodPost, "/machines", spec, &answer)
}

func (c *Client) Terminate(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/machines/"+url.PathEscape(id), nil, nil)
}

func (c *Client) Machines(ctx context.Context, owner string) ([]machine.Machine, error) {
	var answer struct {
		Machines []machine.Machine `json:"machines"`
	}

	return answer.Machines, c.do(ctx, http.MethodGet, "/machines?"+url.Values{"owner": {owner}}.Encode(), nil, &answer)
}

func (c *Client) EnsureNetwork(ctx context.Context, owner string, name string, masquerade bool) (machine.Network, error) {
	var answer struct {
		Network machine.Network `json:"network"`
	}

	body := struct {
		Masquerade bool `json:"masquerade"`
	}{Masquerade: masquerade}

	return answer.Network, c.do(ctx, http.MethodPut, networkPath(owner, name), body, &answer)
}

func (c *Client) RemoveNetwork(ctx context.Context, owner string, name string) error {
	return c.do(ctx, http.MethodDelete, networkPath(owner, name), nil, nil)
}

func networkPath(owner string, name string) string {
	return "/networks/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
}

// do makes one request and reads its answer into answer, if there is one to
// read. A refusal is turned into what it means.
func (c *Client) do(ctx context.Context, method string, path string, body any, answer any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}

		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, launcherAddress+path, payload)
	if err != nil {
		return err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("the launcher could not be reached: %w", err)
	}
	defer response.Body.Close()

	content, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer))
	if err != nil {
		return err
	}

	switch {
	case response.StatusCode == http.StatusConflict:
		return machine.ErrNetworkInUse
	case response.StatusCode == http.StatusBadRequest:
		var refused struct {
			Errors map[string]string `json:"errors"`
		}

		_ = json.Unmarshal(content, &refused)

		return fmt.Errorf("the launcher refused what it was asked: %v", refused.Errors)
	case response.StatusCode >= http.StatusBadRequest:
		return errors.New("the launcher failed: " + string(bytes.TrimSpace(content)))
	}

	if answer == nil {
		return nil
	}

	return json.Unmarshal(content, answer)
}
