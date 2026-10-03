// Package conformance is what a driver has to do to run the workload's tasks.
//
// Every driver runs the same scenarios against a real backend — a docker
// daemon, a vmhost — skipping only what its class declares it cannot do. A
// class's capabilities are honest when it passes: the control plane turns a
// task away by what a class declares, so a class that declares more than it
// does lets a task through to a gap the user never hears about, and one that
// declares less turns away tasks it could run.
//
// It is a package of its own, imported by each driver's tests, rather than a
// test of any one driver, so a new driver is done when Run passes for it.
package conformance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Factory builds the driver under test, for a node. It is asked twice, for two
// nodes, and both have to reach the same backend, as orchestrators that share a
// daemon or a vmhost do.
type Factory func(ctx context.Context, node string) (driver.Driver, error)

// Option changes what a run of the suite assumes about where it runs.
type Option func(*options)

type options struct {
	image     string
	internet  string
	forbidden []string
	timeout   time.Duration
}

const (
	// defaultImage has a shell and busybox's applets, every scenario needs:
	// sh, sleep, seq, httpd, wget, stty, ps, head and yes. It is published for
	// every architecture a class may run on.
	defaultImage = "busybox:1.36"

	// defaultInternet is somewhere on the internet that answers plain HTTP
	// and never sends anybody to HTTPS instead, which busybox's wget cannot
	// check: a connectivity check, which exists for exactly that.
	defaultInternet = "http://connectivitycheck.gstatic.com/generate_204"

	// defaultTimeout is how long one scenario may take, image pulls included.
	defaultTimeout = 3 * time.Minute
)

// WithImage runs the scenarios with another image, which has to have what
// busybox has.
func WithImage(image string) Option {
	return func(o *options) { o.image = image }
}

// WithInternet is somewhere on the internet a public task must reach, and an
// isolated one must not.
func WithInternet(url string) Option {
	return func(o *options) { o.internet = url }
}

// Offline is a backend with no way out to the internet at all, where a public
// task reaching it cannot be asked for. An isolated task still must not.
func Offline() Option {
	return func(o *options) { o.internet = "" }
}

// WithForbidden are addresses, as host:port, a public task must not reach: a
// cloud's metadata service, the platform's own services on a docker network.
func WithForbidden(addresses ...string) Option {
	return func(o *options) { o.forbidden = append(o.forbidden, addresses...) }
}

// WithTimeout is how long one scenario may take.
func WithTimeout(timeout time.Duration) Option {
	return func(o *options) { o.timeout = timeout }
}

// suite is one run of the scenarios: the driver under test for a node of its
// own, and another node's driver on the same backend, to tell the two apart.
type suite struct {
	driver   driver.Driver
	other    driver.Driver
	declared runtime.Capabilities
	options  options

	// node and otherNode are the nodes the two drivers run for. They are
	// named for this run alone, so what an earlier run left behind on the
	// backend is not taken for this one's.
	node      string
	otherNode string
}

// Run runs every scenario against the driver factory builds, skipping what
// declared says the class cannot do. declared is the class's capabilities as
// the driver offers them; the suite checks it offers exactly those.
//
// Everything a scenario makes is taken away when it ends, whether it passed or
// not.
func Run(t *testing.T, factory Factory, declared runtime.Capabilities, opts ...Option) {
	t.Helper()

	o := options{
		image:     defaultImage,
		internet:  defaultInternet,
		forbidden: []string{"169.254.169.254:80"},
		timeout:   defaultTimeout,
	}

	for _, opt := range opts {
		opt(&o)
	}

	run := random()

	s := &suite{
		declared:  declared,
		options:   o,
		node:      "conformance-" + run,
		otherNode: "conformance-" + run + "-other",
	}

	ctx, cancel := context.WithTimeout(t.Context(), o.timeout)
	defer cancel()

	var err error

	s.driver, err = factory(ctx, s.node)
	require.NoError(t, err, "the driver under test cannot be built")
	t.Cleanup(func() { _ = s.driver.Close() })

	s.other, err = factory(ctx, s.otherNode)
	require.NoError(t, err, "a second node's driver cannot be built")
	t.Cleanup(func() { _ = s.other.Close() })

	// what every scenario runs, made ready once rather than raced for.
	require.NoError(t, s.driver.Tasks().EnsureImage(ctx, o.image), "the image cannot be made ready")

	if declared.SupportsNetworkPolicy(network.PolicyIsolated) || declared.SupportsNetworkPolicy(network.PolicyPublic) {
		require.NoError(t, s.driver.Networks().EnsureIsolatedNetwork(ctx), "the isolated network cannot be made")
	}

	t.Run("offer", s.offer)

	scenarios := []struct {
		name string
		run  func(*testing.T)
	}{
		{"lifecycle", s.lifecycle},
		{"listing", s.listing},
		{"exit codes", s.exitCodes},
		{"logs", s.logs},
		{"exec", s.exec},
		{"limits", s.limits},
		{"read only", s.readOnly},
		{"networks", s.networks},
		{"ports", s.ports},
		{"restart policies", s.restartPolicies},
	}

	// they share nothing but the backend, so they run side by side.
	t.Run("scenarios", func(t *testing.T) {
		for _, scenario := range scenarios {
			t.Run(scenario.name, func(t *testing.T) {
				t.Parallel()

				scenario.run(t)
			})
		}
	})
}

// random is a short name no earlier run used.
func random() string {
	return strings.ReplaceAll(uuid.Must(uuid.NewV4()).String(), "-", "")[:8]
}

// context is a scenario's own deadline.
func (s *suite) context(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), s.options.timeout)
	t.Cleanup(cancel)

	return ctx
}

// execution is a run of the suite's image for the suite's node, named for the
// scenario, running command in a shell, with no network until a scenario asks
// for one. change is what the scenario asks for beyond that.
func (s *suite) execution(name string, kind task.Kind, command string, change ...func(*task.Execution)) *task.Execution {
	slug := fmt.Sprintf("cf-%s-%s", name, random()[:5])

	e := &task.Execution{
		Name:     slug,
		Runtime:  s.driver.Class(),
		TaskUUID: uuid.Must(uuid.NewV4()).String(),
		TaskName: name,
		Slug:     slug,
		Kind:     kind,
		NodeName: s.node,
		Image:    s.options.image,
		Command:  []string{"sh", "-c", command},
		ResourceLimits: task.ResourceLimits{
			Cpu:    0.5,
			Memory: 64 << 20,
			Disk:   256 << 20,
		},
		RestartPolicy: "no",
		Networks:      network.Attachments(network.PolicyNone, "", ""),
	}

	for _, c := range change {
		c(e)
	}

	return e
}

// on puts a run on a network policy, alone or as a service of a stack.
func on(policy network.Policy, stackSlug string, service string) func(*task.Execution) {
	return func(e *task.Execution) {
		e.Networks = network.Attachments(policy, stackSlug, service)
	}
}

// create makes a run with a driver, and takes it away again when the scenario
// ends.
func create(t *testing.T, ctx context.Context, d driver.Driver, e *task.Execution) string {
	t.Helper()

	id, err := d.Tasks().Create(ctx, e)
	require.NoError(t, err, "creating %s", e.Name)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := d.Tasks().Delete(ctx, id); err != nil && !errors.Is(err, domain.ErrNotExists) {
			t.Logf("could not take away %s (%s): %v", e.Name, id, err)
		}
	})

	return id
}

// started makes a run and starts it.
func started(t *testing.T, ctx context.Context, d driver.Driver, e *task.Execution) string {
	t.Helper()

	id := create(t, ctx, d, e)
	require.NoError(t, d.Tasks().Start(ctx, id), "starting %s", e.Name)

	return id
}

// waitFor inspects a run until it is what ready says, and is what it was found
// to be then.
func waitFor(t *testing.T, ctx context.Context, d driver.Driver, id string, what string, ready func(task.Execution) bool) task.Execution {
	t.Helper()

	var (
		last task.Execution
		err  error
	)

	for {
		last, err = d.Tasks().Inspect(ctx, id)
		if err == nil && ready(last) {
			return last
		}

		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("%s never became %s: last seen with status %d, exit code %d, restarted %d times (%v)",
				id, what, last.Status, last.ExitCode, last.RestartCount, err)
		}
	}
}

// says waits until a run has written text. A run counts as running from the
// moment its program is started, before the program has done anything at
// all, such as set up how it ends when it is asked to; what the program
// writes once it has is what says it is ready.
func says(t *testing.T, ctx context.Context, d driver.Driver, id string, text string) {
	t.Helper()

	var written bytes.Buffer

	for {
		written.Reset()
		if err := d.Tasks().Logs(ctx, id, &written); err == nil && strings.Contains(written.String(), text) {
			return
		}

		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("%s never wrote %q: it wrote %q", id, text, written.String())
		}
	}
}

func running(e task.Execution) bool {
	return e.Status == task.StatusRunning
}

func ended(e task.Execution) bool {
	return e.Status.Ended()
}

// output is everything a run wrote.
func output(t *testing.T, ctx context.Context, d driver.Driver, id string) string {
	t.Helper()

	var buffer bytes.Buffer
	require.NoError(t, d.Tasks().Logs(ctx, id, &buffer))

	return buffer.String()
}

// job runs a job to its end, and is how it ended and what it wrote.
func job(t *testing.T, ctx context.Context, d driver.Driver, e *task.Execution) (task.Execution, string) {
	t.Helper()

	e.Kind = task.KindJob

	id := started(t, ctx, d, e)
	finished := waitFor(t, ctx, d, id, "ended", ended)

	return finished, output(t, ctx, d, id)
}
