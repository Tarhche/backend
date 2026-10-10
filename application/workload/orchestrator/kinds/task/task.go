// Package task is the task kind's node strategy: what a node does to the
// tasks it runs, and what it says they are doing.
//
// Everything is done through the node's task runtime (task.Runtime), which
// runs each run of a task as an ephemeral VM of its own on the node's engine,
// labelled with what the run was made as: the task's uuid, name, slug, kind
// and owner, which attempt at it the run is, whether it is watched and its
// ttl. What a node knows of a task is what a command carries, the task as the
// control plane recorded it, and what its runs say of themselves.
//
// A command is carried out the way the code runner's tasks always were:
//
//   - create runs a task: a run of it already running is left to run,
//     whatever attempt it is, and what is left of earlier attempts goes, so
//     that the run made now has the task's name and ports; one asked for again
//     takes the run of its attempt that is there already, and one that cannot
//     be started is taken away again, so nothing is left of it to report;
//   - stop and kill stop every run of it: a VM has no grace period to cut
//     short, so the two end it alike;
//   - delete takes every run of it away, stopping a running one first.
//
// Its state is every task this node runs, each as the run of it that runs, or
// otherwise its latest: what its program is doing, and the run itself, with
// what a job has written so far (Run.Output), the ports its VM publishes,
// which the ingress reaches it on, and those that came up. The outputs of a
// node's tasks share a budget, what a beat carries of them between them, a
// task in each of the task kind's heartbeats. A task's terminal is a shell
// inside its running run, anybody's for a task of the guest's, as the page a
// snippet runs on is, and its owner's alone otherwise; its ports are wherever
// its run was published.
package task

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// ReportOutput is the most of their outputs a node's tasks report
	// between them a beat, written out as JSON: a task in each of the task
	// kind's heartbeats, each one NATS message of 1 MiB at most.
	ReportOutput = 512 << 10

	// reading is how many runs' outputs are read at once.
	reading = 8

	// endWait bounds ending a command a terminal's client walked away from.
	endWait = 30 * time.Second

	// failedRun is why a job whose program failed is failed.
	failedRun = "the task failed"

	// prompt is how often what the node's tasks are doing is looked at
	// between beats: a snippet's reader is told it ended, or serves its
	// ports, as soon as that is seen, as often as the code runner always
	// looked.
	prompt = 300 * time.Millisecond
)

// shell is what a terminal is opened with: a busybox image has sh but not
// bash, so sh is the one to reach for.
var shell = []string{"/bin/sh"}

// Node is the task kind's node strategy.
type Node struct {
	runtime  task.Runtime
	nodeName string

	lock sync.Mutex

	// outputs are what each run that has ended wrote, by run: it writes
	// nothing more, so it is read once.
	outputs map[string]string
}

var (
	_ kind.Node[taskKind.Spec, taskKind.Status] = &Node{}
	_ kind.Attacher                             = &Node{}
	_ kind.Exposer                              = &Node{}
	_ kind.Prompt                               = &Node{}
)

// New is the strategy that runs the tasks nodeName holds on runtime.
func New(runtime task.Runtime, nodeName string) *Node {
	return &Node{runtime: runtime, nodeName: nodeName, outputs: make(map[string]string)}
}

// Execute carries out one of a task's commands, and is what it left the task
// as: what its run is doing afterwards.
func (n *Node) Execute(ctx context.Context, t taskKind.Task, action string, _ any) (kind.Outcome[taskKind.Status], error) {
	switch action {
	case taskKind.ActionCreate:
		return n.create(ctx, t)
	case taskKind.ActionStop:
		return n.stop(ctx, t, n.runtime.Stop)
	case taskKind.ActionKill:
		return n.stop(ctx, t, n.runtime.Kill)
	case taskKind.ActionDelete:
		return kind.Outcome[taskKind.Status]{}, n.delete(ctx, t)
	}

	return kind.Outcome[taskKind.Status]{}, fmt.Errorf("%w: a task cannot be %s on its node", kind.ErrUnknownAction, action)
}

// create runs a task, as the attempt at it the control plane counted.
func (n *Node) create(ctx context.Context, t taskKind.Task) (kind.Outcome[taskKind.Status], error) {
	attempt := t.Status.Retries

	runs, err := n.runtime.Of(ctx, t.Metadata.UUID)
	if err != nil {
		return failed(t, err)
	}

	// one of its runs running is what was asked for, whatever attempt it
	// belongs to: a node asked for its tasks again after being away finds them
	// standing.
	if running, found := runningOf(runs); found {
		return n.outcome(ctx, running), nil
	}

	var existing *task.Execution

	for i := range runs {
		if runs[i].Attempt == attempt {
			existing = &runs[i]

			continue
		}

		// what is left of an earlier attempt goes, so this one can have the
		// name and the ports.
		if err := n.runtime.Delete(ctx, runs[i].ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return failed(t, err)
		}
	}

	execution := executionOf(t, attempt, n.nodeName)

	id, err := n.runtime.Create(ctx, &execution)
	if err != nil {
		// asked for twice: the first was cut short after it made the run, and
		// taking the one there is what was wanted either way.
		if existing == nil {
			return failed(t, err)
		}

		id = existing.ID
	}

	if err := n.runtime.Start(ctx, id); err != nil {
		// a run that cannot be started is taken away, so nothing is left of it
		// that its node would report as being on its way.
		_ = n.runtime.Delete(context.WithoutCancel(ctx), id)

		return failed(t, err)
	}

	started, err := n.runtime.Inspect(ctx, id)
	if err != nil {
		return failed(t, err)
	}

	return n.outcome(ctx, started), nil
}

// stop stops every run of a task, by how. A task that has no run here has
// stopped already.
func (n *Node) stop(ctx context.Context, t taskKind.Task, how func(ctx context.Context, executionID string) error) (kind.Outcome[taskKind.Status], error) {
	runs, err := n.runtime.Of(ctx, t.Metadata.UUID)
	if err != nil {
		return kind.Outcome[taskKind.Status]{}, err
	}

	if len(runs) == 0 {
		return kind.Outcome[taskKind.Status]{Status: taskKind.Status{Status: kind.Status{State: taskKind.Stopped}}}, nil
	}

	for _, run := range runs {
		if err := how(ctx, run.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return kind.Outcome[taskKind.Status]{}, err
		}
	}

	stopped, err := n.runtime.Inspect(ctx, current(runs).ID)
	if errors.Is(err, domain.ErrNotExists) {
		return kind.Outcome[taskKind.Status]{Status: taskKind.Status{Status: kind.Status{State: taskKind.Stopped}}}, nil
	} else if err != nil {
		return kind.Outcome[taskKind.Status]{}, err
	}

	return n.outcome(ctx, stopped), nil
}

// delete takes every run of a task away. A running one is stopped first, so
// it ends the way it would if it had been stopped; one that will not stop is
// still taken away. One that was not here is gone already.
func (n *Node) delete(ctx context.Context, t taskKind.Task) error {
	runs, err := n.runtime.Of(ctx, t.Metadata.UUID)
	if err != nil {
		return err
	}

	for _, run := range runs {
		if run.Status == task.StatusRunning {
			_ = n.runtime.Stop(ctx, run.ID)
		}

		if err := n.runtime.Delete(ctx, run.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return err
		}

		n.forget(run.ID)
	}

	return nil
}

// Query reads what a task's run has written, as its runtime has it now.
func (n *Node) Query(ctx context.Context, t taskKind.Task, action string, payload any) (any, error) {
	if action != taskKind.ActionLogs {
		return nil, fmt.Errorf("%w: a task has no %q", kind.ErrUnknownAction, action)
	}

	asked, _ := payload.(taskKind.LogsPayload)

	runs, err := n.runtime.Of(ctx, t.Metadata.UUID)
	if err != nil {
		return nil, err
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("%w: this node holds no run of %q", vm.ErrNotRunning, t.Metadata.UUID)
	}

	var written bytes.Buffer
	if err := n.runtime.Logs(ctx, current(runs).ID, &written); err != nil {
		return nil, err
	}

	lines := linesOf(written.String())

	tail, truncated := asked.Tail, false
	if tail == 0 || tail > noderequest.MaxLogLines {
		tail = noderequest.MaxLogLines
	}

	if uint(len(lines)) > tail {
		lines, truncated = lines[uint(len(lines))-tail:], true
	}

	fitted, cut := reply.Last(lines)

	return taskKind.Logs{Lines: fitted, Truncated: truncated || cut}, nil
}

// State is every task this node runs, each as the run of it that runs, or
// otherwise its latest, with what the jobs among them have written, their
// outputs fitted between them into the heartbeat's share of them.
func (n *Node) State(ctx context.Context) (kind.Report[taskKind.Status], error) {
	held, err := n.runtime.OnNode(ctx, n.nodeName)
	if err != nil {
		return kind.Report[taskKind.Status]{}, err
	}

	n.forgetAllBut(held)

	byTask := make(map[string][]task.Execution)
	for _, run := range held {
		// a run that says of no task is nobody's to speak for.
		if len(run.TaskUUID) == 0 {
			continue
		}

		byTask[run.TaskUUID] = append(byTask[run.TaskUUID], run)
	}

	uuids := slices.Sorted(maps.Keys(byTask))

	runs := make([]task.Execution, len(uuids))
	for i, uuid := range uuids {
		runs[i] = current(byTask[uuid])
	}

	outputs := fitted(n.read(ctx, runs), ReportOutput)

	report := kind.Report[taskKind.Status]{Instances: make([]kind.Observed[taskKind.Status], len(runs))}
	for i, run := range runs {
		status := statusOf(run)
		status.Run.Output = outputs[i]

		report.Instances[i] = kind.Observed[taskKind.Status]{UUID: run.TaskUUID, Status: status}
	}

	return report, nil
}

// Prompt is how often the node's tasks are looked at between beats: whoever
// ran a snippet is waiting on its page to be told it ended, which is told
// as soon as it is seen rather than at the node's next beat.
func (n *Node) Prompt() time.Duration {
	return prompt
}

// Endpoint is where port p of the task a slug names is published on this
// node, or its lowest port when p is none.
func (n *Node) Endpoint(ctx context.Context, slug string, p port.Port) (kind.Endpoint, error) {
	if len(slug) == 0 {
		return kind.Endpoint{}, fmt.Errorf("%w: no slug names a task", domain.ErrNotExists)
	}

	runs, err := n.runtime.BySlug(ctx, slug)
	if err != nil {
		return kind.Endpoint{}, err
	}

	if len(runs) == 0 {
		return kind.Endpoint{}, fmt.Errorf("%w: this node holds no task %q", domain.ErrNotExists, slug)
	}

	run := current(runs)
	if run.Status != task.StatusRunning {
		return kind.Endpoint{}, fmt.Errorf("%w: the task is not running", kind.ErrUnreachable)
	}

	endpoint, found := pick(endpointsOf(run), p)
	if !found {
		return kind.Endpoint{}, fmt.Errorf("%w: the task %q does not expose that port", domain.ErrNotExists, slug)
	}

	return kind.Endpoint{Port: endpoint.Port, Address: endpoint.Address}, nil
}

// Attach opens a terminal in a task's running run.
//
// Whose a run is comes off the run itself: it was labelled with its task's
// owner as it was made, so the node answers without a database and without
// taking anybody's word for it. A run of the guest's is everybody's, as the
// page its snippet runs on is, and so is one that says of no owner, as runs
// made before they said did not; any other is its owner's alone. Somebody who
// may not open it is told it is not there, so knowing a uuid says nothing
// about whether one exists.
func (n *Node) Attach(ctx context.Context, action string, uuid string, owner string) (kind.Session, error) {
	if action != taskKind.ActionAttach {
		return nil, fmt.Errorf("%w: a task has no stream %q", kind.ErrUnknownAction, action)
	}

	runs, err := n.runtime.Of(ctx, uuid)
	if err != nil {
		return nil, err
	}

	if len(runs) == 0 {
		return nil, fmt.Errorf("%w: no task %q of theirs", domain.ErrNotExists, uuid)
	}

	run := current(runs)
	if !opensFor(run.OwnerUUID, owner) {
		return nil, fmt.Errorf("%w: no task %q of theirs", domain.ErrNotExists, uuid)
	}

	if run.Status != task.StatusRunning {
		return nil, fmt.Errorf("%w: the task is not running", kind.ErrUnreachable)
	}

	// the shell outlives this call: it ends when the terminal is closed, not
	// when the request that opened it is done being made.
	opened, err := n.runtime.Exec(context.WithoutCancel(ctx), run.ID, task.ExecOptions{Command: shell, TTY: true})
	if err != nil {
		return nil, err
	}

	return newSession(opened), nil
}

// outcome is what a command left a task as: what its run is doing.
func (n *Node) outcome(ctx context.Context, run task.Execution) kind.Outcome[taskKind.Status] {
	status := statusOf(run)
	status.Run.Output = n.read(ctx, []task.Execution{run})[0]

	return kind.Outcome[taskKind.Status]{Status: status}
}

// read is what each of runs has written, a job's last taskKind.MaxOutput
// bytes of it, a few runs at a time. A service writes nothing here: its log
// is shipped a line at a time. One that cannot be read now, because it has
// not started or its engine did not answer in time, has written nothing yet,
// and is read again next time.
func (n *Node) read(ctx context.Context, runs []task.Execution) []string {
	outputs := make([]string, len(runs))
	slots := make(chan struct{}, reading)

	var waiting sync.WaitGroup

	for i, run := range runs {
		if run.Kind == task.KindService {
			continue
		}

		if output, kept := n.kept(run.ID); kept {
			outputs[i] = output

			continue
		}

		waiting.Go(func() {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()

			var written bytes.Buffer
			if err := n.runtime.Logs(ctx, run.ID, &written); err != nil {
				return
			}

			outputs[i] = last(written.String(), taskKind.MaxOutput)

			if run.Status.Ended() {
				n.keep(run.ID, outputs[i])
			}
		})
	}

	waiting.Wait()

	return outputs
}

func (n *Node) kept(executionID string) (string, bool) {
	n.lock.Lock()
	defer n.lock.Unlock()

	output, kept := n.outputs[executionID]

	return output, kept
}

func (n *Node) keep(executionID string, output string) {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.outputs[executionID] = output
}

func (n *Node) forget(executionID string) {
	n.lock.Lock()
	defer n.lock.Unlock()

	delete(n.outputs, executionID)
}

// forgetAllBut lets go of what was kept of the runs this node no longer
// holds.
func (n *Node) forgetAllBut(held []task.Execution) {
	n.lock.Lock()
	defer n.lock.Unlock()

	maps.DeleteFunc(n.outputs, func(id string, _ string) bool {
		return !slices.ContainsFunc(held, func(run task.Execution) bool { return run.ID == id })
	})
}

// failed is what a create that could not be carried out leaves a task as:
// failed, saying why, with the run it was to be, so that whoever follows the
// task from its node, the code runner, knows whose request it answers.
func failed(t taskKind.Task, err error) (kind.Outcome[taskKind.Status], error) {
	return kind.Outcome[taskKind.Status]{Status: taskKind.Status{
		Status: kind.Status{State: taskKind.Failed, Reason: err.Error()},
		Run: &taskKind.Run{
			Attempt:     t.Status.Retries,
			Name:        t.Metadata.Name,
			Slug:        t.Metadata.Slug,
			Kind:        t.Spec.TaskKind(),
			Interactive: t.Spec.Interactive,
		},
	}}, err
}

// executionOf is the run a task is made as, labelled with what it is, which
// its runtime keeps on it.
func executionOf(t taskKind.Task, attempt int, nodeName string) task.Execution {
	name := t.Metadata.Slug
	if len(name) == 0 {
		name = t.Metadata.Name
	}

	exposed := make(port.PortSet, len(t.Spec.Ports))
	published := make(port.PortMap, len(t.Spec.Ports))

	// every port is published on a host port its node picks, so the workload
	// never keeps track of what is taken on it.
	for _, p := range t.Spec.Ports {
		exposed[p] = struct{}{}
		published[p] = []port.PortBinding{{HostIP: "0.0.0.0"}}
	}

	return task.Execution{
		Name:           name,
		TaskUUID:       t.Metadata.UUID,
		TaskName:       t.Metadata.Name,
		Slug:           t.Metadata.Slug,
		Kind:           t.Spec.TaskKind(),
		NodeName:       nodeName,
		OwnerUUID:      t.Metadata.OwnerUUID,
		Attempt:        attempt,
		Interactive:    t.Spec.Interactive,
		TTL:            t.Spec.TTL,
		Image:          t.Spec.Image,
		Entrypoint:     slices.Clone(t.Spec.Entrypoint),
		Command:        slices.Clone(t.Spec.Command),
		Environment:    slices.Clone(t.Spec.Environment),
		ExposedPorts:   exposed,
		PortBindings:   published,
		NetworkPolicy:  t.Spec.Policy(),
		ResourceLimits: t.Spec.Limits.ResourceLimits(),
	}
}

// statusOf is what a run says its task is doing, in the kind's words, with
// the run itself.
func statusOf(run task.Execution) taskKind.Status {
	state := taskKind.StateOf(run.Status, run.Kind, run.ExitCode)

	status := taskKind.Status{
		Status: kind.Status{State: state},
		Run: &taskKind.Run{
			ID:          run.ID,
			Attempt:     run.Attempt,
			Name:        run.TaskName,
			Slug:        run.Slug,
			Kind:        run.Kind,
			Interactive: run.Interactive,
			Ports:       portsOf(run),
			StartedAt:   run.StartedAt,
			Deadline:    run.Deadline(),
			Endpoints:   endpointsOf(run),
		},
	}

	if run.Status.Ended() {
		status.Run.ExitCode = run.ExitCode
	}

	if state == taskKind.Failed {
		status.Reason = failedRun
	}

	return status
}

// portsOf are the ports a run's VM publishes, lowest first, whether or not
// they are up now: those it serves, as it was made, under a network policy
// that lets anything in, and none otherwise.
func portsOf(run task.Execution) []port.Port {
	return slices.Sorted(maps.Keys(run.ExposedPorts))
}

// endpointsOf are the ports of a run that came up, lowest first, and where
// its node reaches each: a run that is not running serves none.
func endpointsOf(run task.Execution) []taskKind.Endpoint {
	var endpoints []taskKind.Endpoint

	for p, bindings := range run.PortBindings {
		for _, binding := range bindings {
			if binding.HostPort == 0 {
				continue
			}

			endpoints = append(endpoints, taskKind.Endpoint{Port: p, Address: net.JoinHostPort(binding.HostIP, strconv.Itoa(int(binding.HostPort)))})

			break
		}
	}

	slices.SortFunc(endpoints, func(a, b taskKind.Endpoint) int { return cmp.Compare(a.Port, b.Port) })

	return endpoints
}

// pick is the endpoint of the port asked for, or of the lowest one when none
// was, which is what a task with a single port needs no port in its name for.
func pick(endpoints []taskKind.Endpoint, requested port.Port) (taskKind.Endpoint, bool) {
	for _, endpoint := range endpoints {
		if requested == 0 || endpoint.Port == requested {
			return endpoint, true
		}
	}

	return taskKind.Endpoint{}, false
}

// current is the run of a task that speaks for it: the one running, or
// otherwise its latest attempt, and of those the one that started last.
func current(runs []task.Execution) task.Execution {
	if running, found := runningOf(runs); found {
		return running
	}

	return slices.MaxFunc(runs, func(a, b task.Execution) int {
		return cmp.Or(cmp.Compare(a.Attempt, b.Attempt), a.StartedAt.Compare(b.StartedAt))
	})
}

func runningOf(runs []task.Execution) (task.Execution, bool) {
	for _, run := range runs {
		if run.Status == task.StatusRunning {
			return run, true
		}
	}

	return task.Execution{}, false
}

// opensFor reports whether a run labelled as owned by labelled opens a
// terminal for asking.
func opensFor(labelled string, asking string) bool {
	return len(labelled) == 0 || labelled == task.GuestOwnerUUID || labelled == asking
}

// linesOf is what was written, a line at a time.
func linesOf(written string) []string {
	written = strings.TrimSuffix(written, "\n")
	if len(written) == 0 {
		return []string{}
	}

	return strings.Split(written, "\n")
}

// last is the last limit bytes of output, starting at a whole character.
func last(output string, limit int) string {
	if len(output) <= limit {
		return output
	}

	output = output[len(output)-limit:]

	for i := 0; i < len(output) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(output[i]) {
			return output[i:]
		}
	}

	return output
}

// fitted cuts outputs so that, written out as JSON strings, they take no
// more than budget bytes between them. When they fit together each is kept
// whole; otherwise each is given an even share of what the smaller ones
// leave, and one larger than its share keeps its last share of it: the end of
// what a program printed is what says how it ended.
func fitted(outputs []string, budget int) []string {
	sizes := make([]int, len(outputs))
	total := 0

	for i, output := range outputs {
		sizes[i] = encodedSize(output)
		total += sizes[i]
	}

	if total <= budget {
		return outputs
	}

	order := make([]int, len(outputs))
	for i := range order {
		order[i] = i
	}

	slices.SortFunc(order, func(a, b int) int { return cmp.Compare(sizes[a], sizes[b]) })

	cut := slices.Clone(outputs)
	left := budget

	for k, i := range order {
		share := left / (len(order) - k)

		if sizes[i] > share {
			cut[i] = tailWithin(outputs[i], share)
		}

		left -= min(sizes[i], encodedSize(cut[i]))
	}

	return cut
}

// encodedSize is how many bytes s takes written out as a JSON string, at
// most: what HTML-safe encoding escapes is counted as escaped.
func encodedSize(s string) int {
	size := 2

	for _, r := range s {
		size += encodedRune(r)
	}

	return size
}

func encodedRune(r rune) int {
	switch {
	case r == '"' || r == '\\' || r == '\n' || r == '\r' || r == '\t':
		return 2
	case r < 0x20 || r == '<' || r == '>' || r == '&' || r == ' ' || r == ' ' || r == utf8.RuneError:
		return 6
	}

	return utf8.RuneLen(r)
}

// tailWithin is the longest end of s that takes no more than limit bytes
// written out as a JSON string.
func tailWithin(s string, limit int) string {
	size := 2
	start := len(s)

	for start > 0 {
		r, width := utf8.DecodeLastRuneInString(s[:start])

		if size+encodedRune(r) > limit {
			break
		}

		size += encodedRune(r)
		start -= width
	}

	return s[start:]
}

// session is a command running in a run, as a stream action is carried: its
// output, which under a terminal is all there is, and its input.
type session struct {
	exec task.ExecSession
	done chan struct{}
	once sync.Once
}

var _ kind.Session = &session{}

func newSession(exec task.ExecSession) *session {
	return &session{exec: exec, done: make(chan struct{})}
}

func (s *session) Stdin() io.WriteCloser {
	return input{s.exec}
}

func (s *session) Stdout() io.Reader {
	return s.exec
}

// Stderr is empty: under a terminal, everything is on its output.
func (s *session) Stderr() io.Reader {
	return strings.NewReader("")
}

func (s *session) Resize(ctx context.Context, rows uint, cols uint) error {
	return s.exec.Resize(ctx, rows, cols)
}

// Wait waits for the session to be closed: a run's command says nothing of
// how it ended.
func (s *session) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.done:
		return 0, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Close lets go of the command, and then ends it, and everything it started,
// given a moment to finish on its own: the client is gone, and what it left
// running has nothing to show its output to.
func (s *session) Close() error {
	var err error

	s.once.Do(func() {
		close(s.done)

		err = s.exec.Close()

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), endWait)
			defer cancel()

			_ = s.exec.End(ctx)
		}()
	})

	return err
}

// input is a command's input, which closing ends nothing of: the command
// ends with its session.
type input struct {
	exec task.ExecSession
}

func (i input) Write(p []byte) (int, error) {
	return i.exec.Write(p)
}

func (i input) Close() error {
	return nil
}
