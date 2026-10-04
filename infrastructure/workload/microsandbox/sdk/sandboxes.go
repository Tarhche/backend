//go:build microsandbox

package sdk

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

const (
	// kvm is the device the guests' hardware virtualization is reached by.
	kvm = "/dev/kvm"

	// forceWait bounds the kill that follows a stop that did not finish in
	// time, discardWait the removal of what a failed create left,
	// destroyWait microsandbox's own wait while it destroys a sandbox, and
	// staleProbeWait each step of clearStale.
	forceWait      = 10 * time.Second
	discardWait    = 30 * time.Second
	destroyWait    = 10 * time.Second
	staleProbeWait = 10 * time.Second

	// listPage is the most sandboxes microsandbox lists at a time.
	listPage = 100
)

// Sandboxes is microsandbox, driven through its Go SDK from this process.
//
// The sandboxes it makes are detached: their VMs outlive this process, and
// another finds them by name or by label. What does not outlive it is a
// command started through it, because microsandbox kills the commands of a
// client that goes away.
type Sandboxes struct {
	logger *slog.Logger
}

var _ runs.Sandboxes = &Sandboxes{}

// New drives microsandbox as its environment configures it: MSB_HOME, MSB_PATH
// and MSB_LIBKRUNFW_PATH.
func New(logger *slog.Logger) *Sandboxes {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Sandboxes{logger: logger}
}

// Check opens /dev/kvm for reading and writing, resolves the runtime's msb and
// libkrunfw without fetching any, and asks that msb its version. The SDK's own
// RuntimeVersion is no substitute: it is the version of the library the SDK
// embeds, not of the msb it drives.
func (s *Sandboxes) Check(ctx context.Context) (runs.Versions, error) {
	device, err := os.OpenFile(kvm, os.O_RDWR, 0)
	if err != nil {
		return runs.Versions{}, fmt.Errorf("microsandbox: %s does not open for reading and writing: %w", kvm, err)
	}
	device.Close()

	msbPath, err := s.msbPath()
	if err != nil {
		return runs.Versions{}, err
	}

	output, err := exec.CommandContext(ctx, msbPath, "--version").Output()
	if err != nil {
		return runs.Versions{}, fmt.Errorf("microsandbox: %s --version: %w", msbPath, err)
	}

	return runs.Versions{
		SDK:          msb.SDKVersion(),
		Runtime:      runtimeVersion(string(output)),
		Architecture: runtime.GOARCH,
	}, nil
}

// Pull runs msb pull, since the SDK can pull an image only as it makes a
// sandbox. An image that is cached already is not pulled again.
func (s *Sandboxes) Pull(ctx context.Context, reference string) error {
	if reference == "" || strings.HasPrefix(reference, "-") {
		return fmt.Errorf("microsandbox: %q is not an image", reference)
	}

	if _, err := msb.Image.Get(ctx, reference); err == nil {
		return nil
	} else if !msb.IsKind(err, msb.ErrImageNotFound) {
		return fmt.Errorf("microsandbox: look %s up: %w", reference, err)
	}

	msbPath, err := s.msbPath()
	if err != nil {
		return err
	}

	output, err := exec.CommandContext(ctx, msbPath, "pull", "--quiet", reference).CombinedOutput()
	if err != nil {
		return fmt.Errorf("microsandbox: pull %s: %w: %s", reference, err, lastLine(output))
	}

	return nil
}

func (s *Sandboxes) Image(ctx context.Context, reference string) (runs.ImageConfig, bool, error) {
	detail, err := msb.Image.Inspect(ctx, reference)
	if msb.IsKind(err, msb.ErrImageNotFound) {
		return runs.ImageConfig{}, false, nil
	}

	if err != nil {
		return runs.ImageConfig{}, false, fmt.Errorf("microsandbox: inspect %s: %w", reference, err)
	}

	return imageConfigOf(detail), true, nil
}

// Create makes a sandbox and boots it, detached. A create that fails leaves
// nothing behind: microsandbox keeps a sandbox that failed to boot, or whose
// create was cancelled, and the next create of its name would fail on it.
func (s *Sandboxes) Create(ctx context.Context, spec runs.SandboxSpec) (runs.Sandbox, error) {
	options, err := createOptions(spec)
	if err != nil {
		return nil, err
	}

	handle, err := msb.CreateSandbox(ctx, spec.Name, options...)
	if err != nil {
		// one that was there already is somebody else's to remove
		if !msb.IsKind(err, msb.ErrSandboxAlreadyExists) {
			s.discard(spec.Name)
		}

		return nil, fmt.Errorf("microsandbox: create %s: %w", spec.Name, err)
	}

	return &sandbox{name: spec.Name, handle: handle, logger: s.logger}, nil
}

// Start boots a sandbox that is not running: one that was stopped, and one
// that crashed, which is what a restart of the container leaves every running
// sandbox as.
//
// It cannot boot one whose VM's PID, before the container restarted, is
// another process's or a thread's now, since microsandbox 0.7.6 then takes
// that VM for alive (its issue #1642). The PIDs of a new container start over,
// and this process's own threads take the ones the first VMs of the last had,
// so after a restart the oldest sandboxes are the likeliest to be refused.
// clearStale gets past a PID that a short-lived process holds, not one a
// thread does: such a sandbox fails with runs.ErrStuck.
func (s *Sandboxes) Start(ctx context.Context, name string) (runs.Sandbox, error) {
	handle, err := msb.StartSandboxDetached(ctx, name)
	if msb.IsKind(err, msb.ErrSandboxStillRunning) {
		switch s.clearStale(ctx, name) {
		case staleCleared:
			handle, err = msb.StartSandboxDetached(ctx, name)
		case staleStuck:
			return nil, fmt.Errorf("microsandbox: start %s: %w: %w", name, runs.ErrStuck, err)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("microsandbox: start %s: %w", name, err)
	}

	return &sandbox{name: name, handle: handle, logger: s.logger}, nil
}

func (s *Sandboxes) Connect(ctx context.Context, name string) (runs.Sandbox, error) {
	found, err := msb.GetSandbox(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("microsandbox: find %s: %w", name, err)
	}

	handle, err := found.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("microsandbox: connect to %s: %w", name, err)
	}

	return &sandbox{name: name, handle: handle, logger: s.logger}, nil
}

// Stop asks the guest to shut down and kills the VM if it has not within
// timeout, which microsandbox never does by itself. A sandbox that is not
// running, or not there at all, is stopped already.
func (s *Sandboxes) Stop(ctx context.Context, name string, timeout time.Duration) error {
	found, err := msb.GetSandbox(ctx, name)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("microsandbox: find %s: %w", name, err)
	}

	if !running(string(found.Status())) {
		return nil
	}

	stopErr := found.Stop(ctx, msb.WithStopTimeout(timeout))
	if stopErr == nil {
		return nil
	}

	// the kill gets a deadline of its own, since ctx may have run out
	// waiting for the stop
	killCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forceWait)
	defer cancel()

	if err := found.Kill(killCtx); err != nil {
		if current, getErr := msb.GetSandbox(killCtx, name); getErr == nil && !running(string(current.Status())) {
			return nil
		}

		return fmt.Errorf("microsandbox: stop %s: %v; then kill it: %w", name, stopErr, err)
	}

	return nil
}

// Remove destroys a sandbox, killing its VM if it is running. One microsandbox
// takes for running though no VM runs it (see Start) fails with runs.ErrStuck.
func (s *Sandboxes) Remove(ctx context.Context, name string) error {
	found, err := msb.GetSandbox(ctx, name)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("microsandbox: find %s: %w", name, err)
	}

	err = found.Destroy(ctx, msb.WithDestroyForce(), msb.WithDestroyTimeout(destroyWait))
	if err != nil && !msb.IsKind(err, msb.ErrSandboxNotFound) {
		if s.stuck(ctx, name) {
			return fmt.Errorf("microsandbox: remove %s: %w: %w", name, runs.ErrStuck, err)
		}

		return fmt.Errorf("microsandbox: remove %s: %w", name, err)
	}

	return nil
}

func (s *Sandboxes) List(ctx context.Context, labels map[string]string) ([]runs.SandboxInfo, error) {
	var (
		infos  []runs.SandboxInfo
		cursor string
	)

	for {
		options := []msb.SandboxListOption{msb.WithListLimit(listPage)}
		if len(labels) > 0 {
			options = append(options, msb.WithListLabels(labels))
		}

		if cursor != "" {
			options = append(options, msb.WithListCursor(cursor))
		}

		page, err := msb.ListSandboxesWith(ctx, options...)
		if err != nil {
			return nil, fmt.Errorf("microsandbox: list sandboxes: %w", err)
		}

		for _, found := range page.Sandboxes {
			config, err := found.Config()
			if err != nil {
				return nil, fmt.Errorf("microsandbox: read the configuration of %s: %w", found.Name(), err)
			}

			infos = append(infos, runs.SandboxInfo{
				Name:    found.Name(),
				Labels:  config.Labels,
				Running: running(string(found.Status())),
			})
		}

		if page.NextCursor == nil || *page.NextCursor == "" || *page.NextCursor == cursor {
			return infos, nil
		}

		cursor = *page.NextCursor
	}
}

func (s *Sandboxes) Metrics(ctx context.Context) (map[string]runs.Metrics, error) {
	all, err := msb.AllSandboxMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("microsandbox: read the sandboxes' metrics: %w", err)
	}

	metrics := make(map[string]runs.Metrics, len(all))
	for name, m := range all {
		if m != nil {
			metrics[name] = metricsOf(m)
		}
	}

	return metrics, nil
}

// msbPath is the msb the SDK resolves, which MSB_PATH names in the
// workload-microsandbox image. Resolving never fetches one.
func (s *Sandboxes) msbPath() (string, error) {
	resolved, err := msb.ResolveRuntime(msb.RuntimeConfig{})
	if err != nil {
		return "", fmt.Errorf("microsandbox: the runtime's msb and libkrunfw do not resolve: %w", err)
	}

	return resolved.MSBPath, nil
}

// staleness is what clearStale made of a sandbox microsandbox says is
// running.
type staleness int

const (
	// staleUnknown is a sandbox that could not be looked up. Nothing was
	// done to it.
	staleUnknown staleness = iota

	// staleAlive is a sandbox whose guest agent answers: a VM does run it,
	// and it is left alone.
	staleAlive

	// staleCleared is a sandbox no VM ran, whose record is stopped now.
	staleCleared

	// staleStuck is a sandbox no VM runs, which microsandbox would not stop
	// either.
	staleStuck
)

// clearStale sets right the record of a sandbox that microsandbox says is
// running when no VM runs it, and says what it found.
//
// Microsandbox judges whether a sandbox it recorded as running still is by
// the PID its VM had, and nothing else. After a restart of the container,
// that PID can be another process's, or a thread's of this one, and the
// sandbox then looks running, and cannot be started. Its stop judges by the
// lifecycle lock a VM holds instead, and signals nothing that does not hold
// it: stopping a sandbox no VM runs only brings its record to stopped. That
// works while a process holds the PID, and not while a thread does. A sandbox
// whose guest agent answers is running, and is left alone.
func (s *Sandboxes) clearStale(ctx context.Context, name string) staleness {
	found, err := msb.GetSandbox(ctx, name)
	if err != nil {
		return staleUnknown
	}

	probeCtx, cancel := context.WithTimeout(ctx, staleProbeWait)
	defer cancel()

	if live, err := found.Connect(probeCtx); err == nil {
		_ = live.Close()
		return staleAlive
	}

	if err := found.Stop(ctx, msb.WithStopTimeout(staleProbeWait)); err != nil {
		s.logger.Warn("could not set right a sandbox recorded as running that no VM runs", "sandbox", name, "error", err)
		return staleStuck
	}

	return staleCleared
}

// stuck is whether microsandbox takes a sandbox for running though no VM runs
// it: it says the sandbox is running, and no guest agent answers. It is asked
// with a deadline of its own, since ctx may have run out on what failed.
func (s *Sandboxes) stuck(ctx context.Context, name string) bool {
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), staleProbeWait)
	defer cancel()

	found, err := msb.GetSandbox(probeCtx, name)
	if err != nil || !running(string(found.Status())) {
		return false
	}

	live, err := found.Connect(probeCtx)
	if err == nil {
		_ = live.Close()
		return false
	}

	return true
}

// discard removes what a create that failed left behind.
func (s *Sandboxes) discard(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), discardWait)
	defer cancel()

	if err := s.Remove(ctx, name); err != nil {
		s.logger.Warn("could not remove what a failed create left behind", "sandbox", name, "error", err)
	}
}
