package client

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// stopTimeout is how long a run's main process is given to finish on its own
// once it has been asked to stop, before it is killed. It is docker's, so a
// task is stopped the same way on either runtime.
const stopTimeout = 10

// Runtime runs the workload's tasks as microVMs: an execution is a run on the
// workload-microsandbox service, and its ID is the run's.
//
// It is a task.Runtime, so everything that runs, watches, stops or reaches a
// task asks it what it asks docker, and is answered in the same terms.
type Runtime struct {
	client *Client
}

var _ task.Runtime = &Runtime{}

// NewRuntime runs tasks on the service the client reaches.
func NewRuntime(client *Client) *Runtime {
	return &Runtime{client: client}
}

// OnNode is every run the named node holds, whatever state it is in.
func (r *Runtime) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	ctx, span := r.client.span(ctx, "task.list", attribute.String("node.name", nodeName))
	defer span.End()

	executions, err := r.list(ctx, nodeName, nil)

	return executions, trace.RecordError(span, err)
}

// Of is the runs of one task: its latest attempt, and whatever is left of the
// ones before it.
//
// They are looked for among this node's runs. Docker would find a container of
// the task that another node made on the daemon they share; the service keeps
// every node's runs apart, and a node has no business with another's anyway.
func (r *Runtime) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	ctx, span := r.client.span(ctx, "task.list", attribute.String("task.uuid", taskUUID))
	defer span.End()

	if len(taskUUID) == 0 {
		return []task.Execution{}, nil
	}

	executions, err := r.list(ctx, r.client.node, url.Values{api.QueryTask: {taskUUID}})

	return executions, trace.RecordError(span, err)
}

// BySlug is the runs answering to the name a task's ports are served under,
// among this node's runs.
func (r *Runtime) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	ctx, span := r.client.span(ctx, "task.list", attribute.String("task.slug", slug))
	defer span.End()

	if len(slug) == 0 {
		return []task.Execution{}, nil
	}

	executions, err := r.list(ctx, r.client.node, url.Values{api.QuerySlug: {slug}})

	return executions, trace.RecordError(span, err)
}

// list is how the service is asked all three of those: a run carries what it is
// running, and the service narrows a node's runs by it. A node that is not
// named holds nothing, as no container carries an empty node's label.
func (r *Runtime) list(ctx context.Context, nodeName string, narrowed url.Values) ([]task.Execution, error) {
	if len(nodeName) == 0 {
		return []task.Execution{}, nil
	}

	query := url.Values{api.QueryNode: {nodeName}}
	for key, values := range narrowed {
		query[key] = values
	}

	var listed api.RunList
	if err := r.client.call(ctx, request{route: api.RouteListRuns, query: query}, &listed); err != nil {
		return nil, err
	}

	executions := make([]task.Execution, len(listed.Runs))
	for i := range listed.Runs {
		executions[i] = execution(&listed.Runs[i])
	}

	slices.SortStableFunc(executions, newestFirst)

	return executions, nil
}

// EnsureImage makes sure an image is in the service's cache, pulling it if it
// is not, so that a task is timed from when it runs rather than from when its
// image started to arrive. One with no variant for the service's architecture
// is refused, since a microVM runs the host's instruction set.
func (r *Runtime) EnsureImage(ctx context.Context, image string) error {
	ctx, span := r.client.span(ctx, "image.ensure", attribute.String("image", image))
	defer span.End()

	err := r.client.call(ctx, request{
		route:   api.RoutePullImage,
		body:    api.PullRequest{Reference: image},
		timeout: pullTimeout,
	}, nil)

	return trace.RecordError(span, err)
}

// Create records a run of an execution, and boots nothing, as docker create
// starts nothing. What microsandbox cannot run is refused here, before the
// service is asked.
//
// A name the node already uses is refused by the service as name_in_use, after
// which whoever asked takes the run that is there, as it does a container.
func (r *Runtime) Create(ctx context.Context, execution *task.Execution) (string, error) {
	ctx, span := r.client.span(ctx, "task.create",
		attribute.String("image", execution.Image),
		attribute.String("name", execution.Name),
	)
	defer span.End()

	spec, err := runSpec(execution, r.client.node)
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	var created api.Run
	if err := r.client.call(ctx, request{route: api.RouteCreateRun, body: spec}, &created); err != nil {
		return "", trace.RecordError(span, err)
	}

	if len(created.ID) == 0 {
		return "", trace.RecordError(span, errors.New("workload-microsandbox created a run and did not say which"))
	}

	r.client.logger.InfoContext(ctx, "run created", "name", created.Name, "runID", created.ID, "image", created.Image)

	return created.ID, nil
}

// Start boots the run's VM and starts its main process, pulling its image
// first if it is missing. Starting one that runs changes nothing.
func (r *Runtime) Start(ctx context.Context, runID string) error {
	return r.command(ctx, "task.start", api.RouteStartRun, runID, nil, pullTimeout)
}

// Stop asks the run's main process to stop, kills it if it has not within
// stopTimeout seconds, and stops the VM. Stopping one that has ended changes
// nothing.
func (r *Runtime) Stop(ctx context.Context, runID string) error {
	return r.command(ctx, "task.stop", api.RouteStopRun, runID, api.StopRequest{TimeoutSeconds: stopTimeout}, answerTimeout)
}

// Restart stops the run as Stop does and starts it as Start does. The run keeps
// its identity, so its log, its published ports and its name all survive, and
// it is not counted as one of its restart policy's restarts.
func (r *Runtime) Restart(ctx context.Context, runID string) error {
	return r.command(ctx, "task.restart", api.RouteRestartRun, runID, api.StopRequest{TimeoutSeconds: stopTimeout}, pullTimeout)
}

// Kill ends the run's main process at once, without the grace period Stop
// gives it, and stops the VM.
func (r *Runtime) Kill(ctx context.Context, runID string) error {
	return r.command(ctx, "task.kill", api.RouteKillRun, runID, nil, answerTimeout)
}

// Delete takes the run away, killing it first if it is running, with its VM
// and its log. One that is not there is domain.ErrNotExists, which whoever
// asked takes as done.
func (r *Runtime) Delete(ctx context.Context, runID string) error {
	return r.command(ctx, "task.delete", api.RouteDeleteRun, runID, nil, answerTimeout)
}

// command asks the service to do one thing to one run.
func (r *Runtime) command(ctx context.Context, name string, route string, runID string, body any, timeout time.Duration) error {
	ctx, span := r.client.span(ctx, name, attribute.String("task.id", runID))
	defer span.End()

	err := r.client.call(ctx, request{route: route, wildcards: runPath(runID), body: body, timeout: timeout}, nil)

	return trace.RecordError(span, err)
}

// Inspect is everything there is to say about one run.
func (r *Runtime) Inspect(ctx context.Context, runID string) (task.Execution, error) {
	ctx, span := r.client.span(ctx, "task.inspect", attribute.String("task.id", runID))
	defer span.End()

	var inspected api.Run
	if err := r.client.call(ctx, request{route: api.RouteGetRun, wildcards: runPath(runID)}, &inspected); err != nil {
		return task.Execution{}, trace.RecordError(span, err)
	}

	return execution(&inspected), nil
}

// Stats is what a run's VM is using.
//
// A run that is not running uses nothing, and is answered with nothing rather
// than with the service's not_running, as docker answers for a container that
// is not running.
func (r *Runtime) Stats(ctx context.Context, runID string) (task.Stats, error) {
	ctx, span := r.client.span(ctx, "task.stats", attribute.String("task.id", runID))
	defer span.End()

	var stats api.Stats

	err := r.client.call(ctx, request{route: api.RouteRunStats, wildcards: runPath(runID)}, &stats)
	if code(err) == api.CodeNotRunning {
		return task.Stats{}, nil
	}

	if err != nil {
		return task.Stats{}, trace.RecordError(span, err)
	}

	return taskStats(stats), nil
}
