package client

import (
	"context"
	"net/http"
	"net/url"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

func stackPath(uuid string) string {
	return "/api/stacks/" + url.PathEscape(uuid)
}

func (c *Client) Stacks(ctx context.Context, ownerUUID string, vmUUID string, number uint) (workloadControlPlane.Page[stack.Stack], error) {
	var payload stackPagePayload
	query := owned(ownerUUID, url.Values{"vm": {vmUUID}, "page": {page(number)}})

	if err := c.call(ctx, http.MethodGet, c.path("/api/stacks", query), nil, &payload); err != nil {
		return workloadControlPlane.Page[stack.Stack]{}, err
	}

	return payload.toPage(), nil
}

// Stack is one stack and the containers compose made for it, which the
// control plane asks its VM's node for.
func (c *Client) Stack(ctx context.Context, ownerUUID string, uuid string) (workloadControlPlane.StackDetail, error) {
	var payload stackDetailPayload
	if err := c.callWithin(ctx, nodeRequestTimeout, http.MethodGet, c.path(stackPath(uuid), owned(ownerUUID, nil)), nil, &payload); err != nil {
		return workloadControlPlane.StackDetail{}, err
	}

	return workloadControlPlane.StackDetail{
		Stack:        payload.toStack(),
		Containers:   containersOf(payload.Containers),
		VMNotRunning: payload.VMNotRunning,
	}, nil
}

// CreateStack deploys a compose project into the Docker VM the request
// chooses, making that VM first when it says so or when ownerUUID has none.
func (c *Client) CreateStack(ctx context.Context, ownerUUID string, request workloadControlPlane.StackRequest) (workloadControlPlane.CreatedStack, error) {
	body := createStackPayload{Name: request.Name, Compose: request.Compose, VM: newChoicePayload(request.VM)}

	var payload createdStackPayload
	if err := c.call(ctx, http.MethodPost, c.path("/api/stacks", owned(ownerUUID, nil)), body, &payload); err != nil {
		return workloadControlPlane.CreatedStack{}, err
	}

	return workloadControlPlane.CreatedStack{VM: payload.VM.toChosen(), Stack: payload.Stack.toStack()}, nil
}

// DeleteStack takes a stack down and removes it, and its volumes with it when
// removeVolumes says so.
func (c *Client) DeleteStack(ctx context.Context, ownerUUID string, uuid string, removeVolumes bool) error {
	query := url.Values{}
	if removeVolumes {
		query.Set("volumes", "true")
	}

	return c.call(ctx, http.MethodDelete, c.path(stackPath(uuid), owned(ownerUUID, query)), nil, nil)
}

func (c *Client) StartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path(stackPath(uuid)+"/start", owned(ownerUUID, nil)), nil, nil)
}

func (c *Client) StopStack(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path(stackPath(uuid)+"/stop", owned(ownerUUID, nil)), nil, nil)
}

func (c *Client) RestartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return c.call(ctx, http.MethodPost, c.path(stackPath(uuid)+"/restart", owned(ownerUUID, nil)), nil, nil)
}
