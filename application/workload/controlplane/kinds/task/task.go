// Package task is the task kind's control-plane strategy: what a task is
// admitted as, what it is asked for when what it is doing is not what it was
// asked to be, and what is readied before its commands are sent.
//
// A task is admitted with what it asked for checked, as tasks always were,
// and what it left out filled in: a job, isolated, worth the retries its kind
// is usually worth. It is given a slug no task and no VM holds, which is what
// its ports are served under, and placed on one of the nodes that spoke in
// the last few seconds, as the scheduler picks; one admitted while none has
// is placed when it is run, by the reconcile loop, once one has.
//
// What it is asked for after that is decided from what its node last said of
// it:
//
//	kind      expected  observed                     asked
//	any       running   created                      create
//	job       any       completed, stopped, failed   delete: it has run to its end
//	job       running   failed, retries left         create, as its next attempt
//	job       any       running past its ttl         kill
//	service   running   failed or stopped            create, while retries are left
//	any       stopped   running                      stop
//
// A job's ttl is counted from when its run started, as its node says, so one
// that never starts is never stopped for it. A task whose node fell silent is
// failed as node_lost by the reconcile loop, and left to its node, which says
// what became of it once it speaks again; a job its node never comes back for
// is deleted after a while.
//
// A task's log, which a service ships a line at a time, goes with it: it is
// deleted as the task's delete is sent.
package task

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

const (
	// Fresh is how recently a node must have spoken to be given a task: one
	// that has not spoken in that long may not be there to run it.
	Fresh = 3 * time.Second

	// LostAfter is how long a job whose node fell silent is left to its node,
	// before it is given up on and deleted: longer than any job of the code
	// runner's may run.
	LostAfter = 10 * time.Minute

	// nodesLimit is the most nodes a task is placed among.
	nodesLimit = 10

	// maxNameLength keeps a name to something a listing can show.
	maxNameLength = 100
)

// Dependencies are what the strategy places tasks with and reads them
// against.
type Dependencies struct {
	Nodes node.Repository

	// Scheduler picks which of the nodes that spoke lately a task goes to.
	Scheduler task.Scheduler

	// Slugs say which slugs are held already: a task's is unique among tasks
	// and VMs, which share the ingress's hostnames.
	Slugs []slugs.Taken

	// Logs are what services' runs write, kept by the control plane, which
	// go with their tasks. Nothing keeps none.
	Logs task.LogRepository

	// Now is the time the strategy goes by; nothing is the time now.
	Now func() time.Time
}

// Tasks is the task kind's control-plane strategy.
type Tasks struct {
	Dependencies
}

var (
	_ kind.ControlPlane[taskKind.Spec, taskKind.Status] = &Tasks{}
	_ kind.Preparer[taskKind.Spec, taskKind.Status]     = &Tasks{}
)

func New(d Dependencies) *Tasks {
	if d.Now == nil {
		d.Now = time.Now
	}

	return &Tasks{Dependencies: d}
}

// Admit takes in a task somebody asked for: checked, its defaults filled
// in, given a slug, and placed on a node when one has spoken lately.
func (s *Tasks) Admit(ctx context.Context, asked taskKind.Task) (taskKind.Task, domain.ValidationErrors, error) {
	if invalid := validate(asked); len(invalid) > 0 {
		return taskKind.Task{}, invalid, nil
	}

	spec := asked.Spec
	spec.Kind = spec.TaskKind()
	spec.NetworkPolicy = spec.Policy()
	spec.Ports = normalized(spec.Ports)

	retries := spec.Retries()
	spec.MaxRetries = &retries

	name := strings.TrimSpace(asked.Metadata.Name)

	slug, err := slugs.Generate(ctx, name, s.Slugs...)
	if err != nil {
		return taskKind.Task{}, nil, err
	}

	nodeName, err := s.place(ctx)
	if err != nil {
		return taskKind.Task{}, nil, err
	}

	return taskKind.Task{
		Kind: taskKind.Name,
		Metadata: kind.Metadata{
			Name:      name,
			Slug:      slug,
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    asked.Metadata.Labels,
			Node:      nodeName,
		},
		Spec:   spec,
		Status: taskKind.Status{Status: kind.Status{State: taskKind.Created, Expected: taskKind.Running}},
	}, nil, nil
}

// Reconcile is what a task is asked for, given what it was asked to be and
// what it was last seen doing.
func (s *Tasks) Reconcile(_ context.Context, t taskKind.Task) ([]kind.Intent, error) {
	state, expected := t.Status.State, t.Status.Expected
	job := t.Spec.TaskKind() == task.KindJob

	switch {
	case state == taskKind.Created && expected == taskKind.Running:
		return intent(taskKind.ActionCreate, "it was admitted, and is to be run")

	case state == taskKind.Failed && t.Status.Reason == reconcile.ReasonNodeLost:
		// its node says what became of it once it speaks again; a job it never
		// comes back for is given up on.
		if job && s.Now().Sub(t.Status.Since) >= LostAfter {
			return intent(taskKind.ActionDelete, "its node fell silent, and never said what became of it")
		}

		return nil, nil

	case job && taskKind.Ended(state):
		if state == taskKind.Failed && expected == taskKind.Running && retriesLeft(t) {
			return intent(taskKind.ActionCreate, "it failed, and is worth another attempt")
		}

		return intent(taskKind.ActionDelete, "it has run to its end")

	case !job && expected == taskKind.Running && (state == taskKind.Failed || state == taskKind.Stopped):
		if !retriesLeft(t) {
			return nil, nil
		}

		return intent(taskKind.ActionCreate, "it ended while it was expected running, and is worth another attempt")

	case job && (state == taskKind.Running || state == taskKind.Restarting) && outlived(t, s.Now()):
		return intent(taskKind.ActionKill, "it has run for longer than its ttl")

	case expected == taskKind.Stopped && (state == taskKind.Running || state == taskKind.Restarting):
		return intent(taskKind.ActionStop, "it runs while it was expected stopped")
	}

	return nil, nil
}

// Apply refuses every action, since a task has none that runs in the
// control plane.
func (s *Tasks) Apply(_ context.Context, _ taskKind.Task, action string, _ any) (taskKind.Task, domain.ValidationErrors, error) {
	return taskKind.Task{}, nil, fmt.Errorf("%w: a task has no %q run in the control plane", kind.ErrUnknownAction, action)
}

// Prepare readies a task's commands before they are sent. A task on no node
// is placed before it is run, and a run asked for again after the last one
// ended is the next attempt, which its node runs anew; one that finds no node
// to go to is left as it was, and run once a node has spoken. A task's log
// goes as its delete is sent.
func (s *Tasks) Prepare(ctx context.Context, t taskKind.Task, action string, _ any) (taskKind.Task, domain.ValidationErrors, error) {
	switch action {
	case taskKind.ActionCreate:
		if len(t.Metadata.Node) == 0 {
			nodeName, err := s.place(ctx)
			if err != nil || len(nodeName) == 0 {
				return t, nil, err
			}

			t.Metadata.Node = nodeName
		}

		if taskKind.Ended(t.Status.State) {
			t.Status.Retries++
		}

	case taskKind.ActionDelete:
		if s.Logs != nil {
			if err := s.Logs.DeleteByTask(ctx, t.Metadata.UUID); err != nil {
				return taskKind.Task{}, nil, err
			}
		}
	}

	return t, nil, nil
}

// place is the node a task goes to: one of those that spoke lately, as the
// scheduler picks, or none when none has.
func (s *Tasks) place(ctx context.Context) (string, error) {
	nodes, err := s.Nodes.GetAll(ctx, 0, nodesLimit)
	if err != nil {
		return "", err
	}

	now := s.Now()

	healthy := make([]node.Node, 0, len(nodes))
	for _, n := range nodes {
		if now.Sub(n.LastHeartbeatAt) <= Fresh {
			healthy = append(healthy, n)
		}
	}

	if len(healthy) == 0 {
		return "", nil
	}

	return s.Scheduler.Pick(healthy).Name, nil
}

func intent(action string, reason string) ([]kind.Intent, error) {
	return []kind.Intent{{Action: action, Reason: reason}}, nil
}

// retriesLeft reports whether a task is worth asking for again.
func retriesLeft(t taskKind.Task) bool {
	allowed := t.Spec.Retries()

	return allowed == task.RetryForever || t.Status.Retries < allowed
}

// outlived reports whether a job has run for longer than its ttl, counted
// from when its run started, as its node says. One that has not started, or
// may run as long as it likes, has nothing to outlive.
func outlived(t taskKind.Task, now time.Time) bool {
	if t.Spec.TTL <= 0 || t.Status.Run == nil || t.Status.Run.StartedAt.IsZero() {
		return false
	}

	return now.Sub(t.Status.Run.StartedAt) > t.Spec.TTL
}

// normalized is ports sorted, each once.
func normalized(ports []port.Port) []port.Port {
	sorted := slices.Clone(ports)
	slices.Sort(sorted)

	return slices.Compact(sorted)
}

// validate is what is wrong with a task as it was asked for, as tasks were
// always held to.
func validate(asked taskKind.Task) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)
	spec := asked.Spec

	switch name := strings.TrimSpace(asked.Metadata.Name); {
	case len(name) == 0:
		invalid["name"] = "required_field"
	case len(name) > maxNameLength:
		invalid["name"] = "invalid_name"
	}

	if len(strings.TrimSpace(spec.Image)) == 0 {
		invalid["image"] = "required_field"
	}

	if spec.Limits.CPU <= 0 {
		invalid["limits.cpu"] = "required_field"
	}

	switch {
	case spec.Limits.Memory == 0:
		invalid["limits.memory"] = "required_field"

	// a VM cannot be made with less. Every task is asked for through here,
	// so this is where it is told so, rather than on whichever node it would
	// have been given to.
	case spec.Limits.Memory < task.MinMemory:
		invalid["limits.memory"] = "memory_below_minimum"
	}

	if spec.Limits.Disk == 0 {
		invalid["limits.disk"] = "required_field"
	}

	if len(spec.Kind) > 0 && !spec.Kind.IsValid() {
		invalid["kind"] = "invalid_value"
	}

	if spec.MaxRetries != nil && *spec.MaxRetries < task.RetryForever {
		invalid["max_retries"] = "invalid_value"
	}

	switch {
	case spec.TTL < 0:
		invalid["ttl"] = "invalid_value"

	// a service runs until it is stopped, so there is no run for a limit to
	// bound: asking for one is asking for something else.
	case spec.TTL > 0 && spec.TaskKind() != task.KindJob:
		invalid["ttl"] = "ttl_requires_a_job"
	}

	policy := spec.Policy()
	if !policy.IsValid() {
		invalid["network_policy"] = "invalid_network_policy"
	}

	if slices.Contains(spec.Ports, 0) {
		invalid["ports"] = "invalid_value"
	}

	// a task with no network has nothing to publish a port on.
	if len(spec.Ports) > 0 && policy.IsValid() && !policy.AllowsPorts() {
		invalid["ports"] = "ports_require_network"
	}

	return invalid
}
