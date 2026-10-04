//go:build linux

// Package agent is the init of every microVM vmhost boots.
//
// The kernel runs it as /init from an initramfs vmhost builds
// (infrastructure/workload/vmm/initrd), so it is there before anything else is
// and whatever image the task runs: nothing has to be put into an image for it
// to run in a machine. It puts the task's root together from the machine's
// disks, brings up its network, and runs the task, chrooted into that root
// and in a cgroup of its own — and, since it is init, it collects every
// process that ends.
//
// It takes its orders over HTTP on a vsock port, which only vmhost reaches,
// through the socket the machine's firecracker exposes on the host. What it is
// told, and what it answers, is domain/workload/guest; every answer carries
// the protocol's version, so a vmhost that adopts a machine booted by an older
// one can tell whether it understands the agent inside.
//
// What the agent does to the machine itself — mounting, its network, its name,
// its clock, turning it off — is behind an interface, and so are the cgroups
// it keeps the task in, so that its handlers can be served on TCP by a test
// that is nobody's init.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

const (
	// maxRequest bounds what the agent reads of a request.
	maxRequest = 1 << 20

	// clockSkew is how far the machine's clock may be from the host's
	// before it is set from it.
	clockSkew = time.Second

	// emptyTimeout is how long what is being ended is given to be gone.
	emptyTimeout = 5 * time.Second

	// drainTimeout is how long a task's output is read after it ends, for
	// whatever it wrote last.
	drainTimeout = 5 * time.Second

	// readHeaderTimeout bounds how long a request may take to say what it
	// is. Only vmhost connects, but a connection that says nothing would
	// otherwise be held forever.
	readHeaderTimeout = 10 * time.Second

	// answerTime is how long an answer that the machine is being turned off
	// is given to be on its way before it is.
	answerTime = 100 * time.Millisecond

	// shutdownGrace is how long the task is given to end on its own when
	// the machine is asked to stop by a signal rather than by vmhost, which
	// stops the task itself first.
	shutdownGrace = 10 * time.Second
)

// errNotInit is the agent being run as anything but a machine's init.
var errNotInit = errors.New("the agent is a machine's init, and runs as nothing else")

// dirs are where the agent puts the task's root together and keeps what it
// writes for it.
type dirs struct {
	// root is the task's root, which the task's processes are chrooted
	// into; "/" is no chroot at all.
	root string

	// files is where the agent keeps the files it binds into the root.
	files string
}

// Agent is a machine's init.
type Agent struct {
	logger  *slog.Logger
	machine machine
	dirs    dirs
	confine confinement
	logs    *ring

	lock sync.Mutex

	// config is what the machine was told it is, once it has been, and
	// broken why it could not be made that.
	config *guest.Config
	broken error

	// process is how the task was last started, and status what became of
	// it. ended is closed when the current run ends.
	process *guest.Process
	status  guest.Status
	pid     int
	ended   chan struct{}

	execs map[string]*execSession

	cpu cpuSample

	turningOff sync.Once
}

// New builds the init of the machine it runs in.
func New(logger *slog.Logger) *Agent {
	return newAgent(logger, &vm{logger: logger}, dirs{root: rootDir, files: filesDir}, newProcessGroups())
}

// newAgent builds an agent that does what it does to the machine through m,
// puts the task's root in d, and keeps what it starts in c.
func newAgent(logger *slog.Logger, m machine, d dirs, c confinement) *Agent {
	children.start()

	return &Agent{
		logger:  logger,
		machine: m,
		dirs:    d,
		confine: c,
		logs:    newRing(defaultLogBytes),
		status:  guest.Status{State: guest.StateCreated},
		ended:   make(chan struct{}),
		execs:   make(map[string]*execSession),
	}
}

// Run makes the machine usable and takes orders until ctx is done.
func (a *Agent) Run(ctx context.Context) error {
	if os.Getpid() != 1 {
		return errNotInit
	}

	confine, err := boot(a.logger)
	if err != nil {
		return err
	}

	a.confine = confine

	a.handleSignals()

	listener, err := listenVsock(guest.Port, fromHost)
	if err != nil {
		return err
	}

	a.logger.Info("the machine is up, and waiting to be told what it is")

	return a.serve(ctx, listener)
}

// serve answers requests on listener until ctx is done.
func (a *Agent) serve(ctx context.Context, listener net.Listener) error {
	server := &http.Server{
		Handler:           a.routes(),
		ReadHeaderTimeout: readHeaderTimeout,
		ErrorLog:          slog.NewLogLogger(a.logger.Handler(), slog.LevelWarn),
	}

	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

// routes is everything the agent answers, on whatever listener it is served.
func (a *Agent) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(guest.RouteHealth, func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc(guest.RouteConfig, a.configure)
	mux.HandleFunc(guest.RouteHosts, a.setHosts)
	mux.HandleFunc(guest.RouteStart, a.start)
	mux.HandleFunc(guest.RouteStatus, a.processStatus)
	mux.HandleFunc(guest.RouteWait, a.wait)
	mux.HandleFunc(guest.RouteSignal, a.signal)
	mux.HandleFunc(guest.RouteStop, a.stop)
	mux.HandleFunc(guest.RouteLogs, a.followLogs)
	mux.HandleFunc(guest.RouteStats, a.stats)
	mux.HandleFunc(guest.RouteExec, a.exec)
	mux.HandleFunc(guest.RouteEndExec, a.endExec)
	mux.HandleFunc(guest.RouteDial, a.dial)
	mux.HandleFunc(guest.RoutePowerOff, a.powerOff)

	return versioned(mux)
}

// versioned says which version of the protocol the agent speaks on every
// answer it gives, refusals and connections taken over included: a vmhost
// that adopts a machine another one booted reads it from whatever it asks
// first.
func versioned(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(guest.VersionHeader, guest.ProtocolVersion)

		next.ServeHTTP(rw, r)
	})
}

// configure tells the machine what it is. It is told once: the same thing
// told again is taken as it was, and anything else is refused. A machine that
// could not be made what it was told is not told again, since half of it may
// already be what it was told: whoever booted it gives up on it instead.
func (a *Agent) configure(rw http.ResponseWriter, r *http.Request) {
	var config guest.Config
	if !decode(rw, r, &config) {
		return
	}

	if len(config.Root.Image) == 0 {
		http.Error(rw, "a machine's root is made from an image, and none was named", http.StatusBadRequest)

		return
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	if a.broken != nil {
		http.Error(rw, "the machine could not be made what it was told: "+a.broken.Error(), http.StatusInternalServerError)

		return
	}

	if a.config != nil {
		told := *a.config
		told.Now, config.Now = time.Time{}, time.Time{}

		if reflect.DeepEqual(told, config) {
			rw.WriteHeader(http.StatusNoContent)

			return
		}

		http.Error(rw, "the machine has already been told what it is", http.StatusConflict)

		return
	}

	if err := a.apply(config); err != nil {
		a.broken = err
		a.logger.Error("the machine could not be made what it was told", "error", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)

		return
	}

	a.config = &config
	a.logger.Info("the machine is what it was told", "hostname", config.Hostname, "read_only", config.Root.ReadOnly(), "interfaces", len(config.Interfaces))

	rw.WriteHeader(http.StatusNoContent)
}

// apply makes the machine what config says: its clock, its root, its network
// and its name, and the files the task finds them in. The lock is held.
func (a *Agent) apply(config guest.Config) error {
	if !config.Now.IsZero() && absolute(time.Since(config.Now)) > clockSkew {
		if err := a.machine.setClock(config.Now); err != nil {
			a.logger.Warn("the machine's clock could not be set", "error", err)
		}
	}

	if err := a.machine.mountRoot(config.Root); err != nil {
		return err
	}

	if err := a.machine.configureInterfaces(config.Interfaces); err != nil {
		return err
	}

	if len(config.Hostname) > 0 {
		if err := a.machine.setHostname(config.Hostname); err != nil {
			return err
		}
	}

	if err := writeFiles(a.dirs.files, config); err != nil {
		return err
	}

	return a.machine.finishRoot(config.Root.ReadOnly())
}

// setHosts replaces the names the machine's neighbours answer to. The file is
// rewritten where it is, so the task sees the change without anything being
// mounted again.
func (a *Agent) setHosts(rw http.ResponseWriter, r *http.Request) {
	var hosts []guest.Host
	if !decode(rw, r, &hosts) {
		return
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	if a.config == nil {
		http.Error(rw, "the machine has not been told what it is", http.StatusConflict)

		return
	}

	a.config.Hosts = hosts

	if err := writeInPlace(a.dirs.files+"/hosts", hostsFile(a.config.Hostname, a.config.Interfaces, hosts)); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// powerOff turns the machine off once the answer is on its way. Turning it off
// ends the firecracker process holding it, which is how the host learns it is
// done. vmhost stops the task before it asks, so whatever still runs is ended
// at once.
func (a *Agent) powerOff(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusAccepted)

	if flusher, ok := rw.(http.Flusher); ok {
		flusher.Flush()
	}

	go a.turnOff(0)
}

// turnOff ends the task, giving it grace to end on its own, and turns the
// machine off, once however often it is asked.
func (a *Agent) turnOff(grace time.Duration) {
	a.turningOff.Do(func() {
		time.Sleep(answerTime)

		a.lock.Lock()
		pid, ended, running := a.pid, a.ended, a.status.State == guest.StateRunning
		a.lock.Unlock()

		if running && grace > 0 {
			_ = syscall.Kill(pid, syscall.SIGTERM)

			select {
			case <-ended:
			case <-time.After(grace):
			}
		}

		if err := a.confine.remove("", emptyTimeout); err != nil {
			a.logger.Warn("what the task started could not all be ended", "error", err)
		}

		a.logger.Info("the machine is turned off")
		a.machine.powerOff()
	})
}

// handleSignals keeps the machine up whatever signals its init is sent.
//
// The kernel sends init only the signals it has a handler for, and the Go
// runtime has one for all of them, which ends the program on most: init
// ending is the kernel panicking. So the ones that would end it are caught
// rather than left to the runtime. They are caught, not ignored, because what
// init ignores the processes it starts ignore too. Being asked to stop —
// ctrl-alt-del, which the kernel turns into an interrupt for init, or a
// terminate — turns the machine off in order; anything else is noted and
// let be.
func (a *Agent) handleSignals() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	stray := make(chan os.Signal, 1)
	signal.Notify(stray, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGABRT, syscall.SIGUSR1, syscall.SIGUSR2)

	go func() {
		for {
			select {
			case received := <-stop:
				a.logger.Info("the machine was asked to stop", "signal", received.String())

				go a.turnOff(shutdownGrace)
			case received := <-stray:
				a.logger.Warn("init was sent a signal, and goes on", "signal", received.String())
			}
		}
	}()
}

// decode reads a request's JSON body into v, answering the request itself when
// it cannot. What follows the JSON is read and let go of, so that nothing of
// the request is left on a connection that is then taken over.
func decode(rw http.ResponseWriter, r *http.Request, v any) bool {
	body := io.LimitReader(r.Body, maxRequest)

	if err := json.NewDecoder(body).Decode(v); err != nil {
		http.Error(rw, "that is not what was expected: "+err.Error(), http.StatusBadRequest)

		return false
	}

	_, _ = io.Copy(io.Discard, body)

	return true
}

// answer writes v as a request's JSON answer.
func answer(rw http.ResponseWriter, v any) {
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(v)
}

func absolute(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
}
