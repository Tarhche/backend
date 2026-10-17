package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
)

// A task is a kind the control plane runs, so it is reached through the
// control plane's resource API, under the kind's plural, as a manifest: the
// code runner runs every snippet as one, reads one back and takes it away.
const tasksPath = "/api/" + taskKind.Plural

func taskPath(uuid string) string {
	return tasksPath + "/" + url.PathEscape(uuid)
}

// askedTask is a task somebody asks for: what of its manifest is theirs to
// say.
type askedTask struct {
	Kind     string        `json:"kind"`
	Metadata kind.Metadata `json:"metadata"`
	Spec     taskKind.Spec `json:"spec"`
}

// commandedTask is what asking for a task came to, as the resource API
// answers.
type commandedTask struct {
	Resource *taskKind.Task `json:"resource"`
}

// RunTask asks for a task to be run, for ownerUUID: admitted as the control
// plane admits any resource, and asked of the node it is placed on.
func (c *Client) RunTask(ctx context.Context, ownerUUID string, request workloadControlPlane.TaskRequest) (taskKind.Task, error) {
	asked := askedTask{
		Kind:     taskKind.Name,
		Metadata: kind.Metadata{Name: request.Name},
		Spec:     request.Spec,
	}

	var payload commandedTask
	if err := c.call(ctx, http.MethodPost, c.path(tasksPath, owned(ownerUUID, nil)), asked, &payload); err != nil {
		return taskKind.Task{}, err
	}

	if payload.Resource == nil {
		return taskKind.Task{}, errors.New("the workload answered with no task")
	}

	return *payload.Resource, nil
}

// Task is one task the workload holds, whoever owns it.
func (c *Client) Task(ctx context.Context, uuid string) (taskKind.Task, error) {
	var manifest taskKind.Task
	if err := c.call(ctx, http.MethodGet, c.path(taskPath(uuid), nil), nil, &manifest); err != nil {
		return taskKind.Task{}, err
	}

	return manifest, nil
}

// DeleteTask removes a task whether or not it is still running: a delete is
// a request to have it gone, which its node takes it away for.
func (c *Client) DeleteTask(ctx context.Context, uuid string) error {
	return c.call(ctx, http.MethodDelete, c.path(taskPath(uuid), nil), nil, nil)
}
