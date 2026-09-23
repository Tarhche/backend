//go:build linux

// Package agent is the init of every microVM the runner starts.
//
// The kernel runs it as /init from an initramfs the orchestrator builds, so
// it is there before anything else is and whatever image the task runs:
// nothing has to be put into an image for it to run in a machine. It puts the
// task's root together from the machine's disks, brings up its network, and
// runs the task — and, since it is init, it collects every process that ends.
//
// It takes its orders on a vsock port, which only the orchestrator holding the
// machine can reach. See the guest package for what it is told.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/vsock"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

const (
	// maxRequest bounds what the agent reads of a request.
	maxRequest = 1 << 20

	// clockSkew is how far the machine's clock may be from the host's
	// before it is set from it.
	clockSkew = time.Second

	// emptyTimeout is how long a cgroup being emptied is given.
	emptyTimeout = 5 * time.Second

	// drainTimeout is how long a task's output is read after it ends, for
	// whatever it wrote last.
	drainTimeout = 5 * time.Second
)

// Agent is a machine's init.
type Agent struct {
	logger *slog.Logger
	reaper *reaper
	logs   *ring

	lock sync.Mutex

	// config is what the machine was told it is, once it has been.
	config *guest.Config

	// process is how the task was last started, and status what became of
	// it. ended is closed when the current run ends.
	process *guest.Process
	status  guest.Status
	pid     int
	ended   chan struct{}

	execs map[string]*execSession

	cpu cpuSample
}

// New builds a machine's init.
func New(logger *slog.Logger) *Agent {
	return &Agent{
		logger: logger,
		reaper: newReaper(),
		logs:   newRing(defaultLogBytes),
		status: guest.Status{State: guest.StateCreated},
		ended:  make(chan struct{}),
		execs:  make(map[string]*execSession),
	}
}

// Run makes the machine usable and takes orders until ctx is done.
func (a *Agent) Run(ctx context.Context) error {
	if os.Getpid() != 1 {
		return errors.New("the agent is a machine's init, and runs as nothing else")
	}

	if err := boot(); err != nil {
		return err
	}

	go a.reaper.run(ctx)

	quiet := logrus.New()
	quiet.Out = io.Discard

	listener, err := vsock.Listener(ctx, logrus.NewEntry(quiet), guest.Port)
	if err != nil {
		return err
	}

	server := &http.Server{
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	a.logger.Info("the machine is up, and waiting to be told what it is")

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (a *Agent) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("PUT /config", a.configure)
	mux.HandleFunc("PUT /hosts", a.setHosts)
	mux.HandleFunc("POST /process", a.start)
	mux.HandleFunc("GET /process", a.processStatus)
	mux.HandleFunc("GET /process/wait", a.wait)
	mux.HandleFunc("POST /process/signal", a.signal)
	mux.HandleFunc("POST /process/stop", a.stop)
	mux.HandleFunc("GET /logs", a.followLogs)
	mux.HandleFunc("GET /stats", a.stats)
	mux.HandleFunc("POST /exec", a.exec)
	mux.HandleFunc("POST /exec/{id}/end", a.endExec)
	mux.HandleFunc("POST /dial", a.dial)
	mux.HandleFunc("POST /poweroff", a.powerOff)

	return mux
}

// configure tells the machine what it is. It is told once: the same thing
// told again is taken as it was, and anything else is refused.
func (a *Agent) configure(rw http.ResponseWriter, r *http.Request) {
	var config guest.Config
	if !decode(rw, r, &config) {
		return
	}

	a.lock.Lock()
	defer a.lock.Unlock()

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
		a.logger.Error("the machine could not be made what it was told", "error", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)

		return
	}

	a.config = &config

	rw.WriteHeader(http.StatusNoContent)
}

// apply makes the machine what config says: its clock, its root, its network
// and its name.
func (a *Agent) apply(config guest.Config) error {
	if !config.Now.IsZero() && absolute(time.Since(config.Now)) > clockSkew {
		now := unix.NsecToTimeval(config.Now.UnixNano())
		if err := unix.Settimeofday(&now); err != nil {
			a.logger.Warn("the machine's clock could not be set", "error", err)
		}
	}

	if err := mountRoot(config.Root); err != nil {
		return err
	}

	if err := configureInterfaces(config.Interfaces); err != nil {
		return err
	}

	if len(config.Hostname) > 0 {
		if err := unix.Sethostname([]byte(config.Hostname)); err != nil {
			return err
		}
	}

	if err := writeFiles(config); err != nil {
		return err
	}

	return bindFiles(!config.Root.ReadOnly())
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

	if err := writeInPlace(filesDir+"/hosts", hostsFile(a.config.Hostname, a.config.Interfaces, hosts)); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// powerOff turns the machine off once the answer is on its way. Turning it off
// ends the firecracker process holding it, which is how the host learns it is
// done.
func (a *Agent) powerOff(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusAccepted)

	if flusher, ok := rw.(http.Flusher); ok {
		flusher.Flush()
	}

	go func() {
		time.Sleep(100 * time.Millisecond)

		_ = removeCgroup(taskCgroup, emptyTimeout)
		unix.Sync()
		unmountRoot()
		unix.Sync()

		// firecracker has no power button: a reset is what ends it.
		_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
	}()
}

// decode reads a request's JSON body into v, answering the request itself when
// it cannot.
func decode(rw http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequest)).Decode(v); err != nil {
		http.Error(rw, "that is not what was expected: "+err.Error(), http.StatusBadRequest)

		return false
	}

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

// Halt turns the machine off at once, for when there is nothing left to run it
// for.
func Halt() {
	unix.Sync()
	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}
