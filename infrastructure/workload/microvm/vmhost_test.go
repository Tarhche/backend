package microvm

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// fakeVMHost is vmhost as the driver sees it: the routes of domain/workload/vm,
// served on a unix socket by an httptest server and answered from memory.
//
// It speaks nothing but what the contract says — its routes are the route
// constants, its answers the contract's types, its refusals vm.Describe's — so
// what the driver does against it is what it does against the real vmhost,
// which is built to the same contract.
type fakeVMHost struct {
	t        *testing.T
	endpoint string
	server   *httptest.Server
	dir      string

	lock sync.Mutex

	info     vm.Info
	vms      []vm.VM
	networks map[string]vm.Network
	images   []vm.Image
	logs     map[string][]vm.LogLine
	live     map[string]chan vm.LogLine
	stats    map[string]vm.Stats

	// statsRefused is what asking what a VM uses answers instead.
	statsRefused map[string]error

	// inUse is how many more times removing a network is refused because VMs
	// are still plugged into it.
	inUse map[string]int

	// room is the memory, in bytes, VMs may still be given. Zero is no limit.
	room uint64

	// refusing is what a route answers instead of doing anything.
	refusing map[string]error

	// wrapped answers listings as {"items": […]} rather than as the list
	// itself, and unfiltered ignores the label filters a listing names.
	wrapped    bool
	unfiltered bool

	// asked is every request made, as "METHOD /path?query".
	asked []string

	// execs, ends and resizes are the commands it was asked to run, how it was
	// asked to end them, and the terminal sizes it was told.
	execs   []guest.Exec
	ends    map[string]guest.EndExec
	resizes []string

	// dialTo is where a connection to a VM's port is carried.
	dialTo string

	hijacked []net.Conn
	made     int
}

func newFakeVMHost(t *testing.T) *fakeVMHost {
	t.Helper()

	f := &fakeVMHost{
		t: t,
		info: vm.Info{
			Version:      "1.2.3",
			Hypervisor:   "firecracker",
			GuestVersion: guest.ProtocolVersion,
			Architecture: "arm64",
			ProcessMode:  "systemd",
			Healthy:      true,
			Capabilities: runtime.Capabilities{
				Isolation:       runtime.IsolationMicroVM,
				NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
				StackNetworks:   true,
				ReadOnlyRoot:    true,
				DiskLimit:       true,
				TTY:             true,
				RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
				MinMemory:       128 << 20,
				MaxMemory:       2 << 30,
				MaxCPU:          4,
				Architectures:   []string{"arm64"},
			},
			Capacity: runtime.Capacity{CPU: 8, Memory: 12 << 30, AllocatedMemory: 1 << 30, Disk: 100 << 30, Reserved: true},
		},
		networks: map[string]vm.Network{
			vm.PublicNetwork: {Name: vm.PublicNetwork, Subnet: "10.250.0.0/24", Gateway: "10.250.0.1", Masquerade: true},
		},
		logs:         make(map[string][]vm.LogLine),
		live:         make(map[string]chan vm.LogLine),
		stats:        make(map[string]vm.Stats),
		statsRefused: make(map[string]error),
		inUse:        make(map[string]int),
		refusing:     make(map[string]error),
		ends:         make(map[string]guest.EndExec),
	}

	mux := http.NewServeMux()

	f.handle(mux, vm.RouteInfo, f.getInfo)
	f.handle(mux, vm.RoutePrepareImage, f.prepareImage)
	f.handle(mux, vm.RouteImages, f.listImages)
	f.handle(mux, vm.RouteDeleteImage, f.deleteImage)
	f.handle(mux, vm.RouteEnsureNetwork, f.ensureNetwork)
	f.handle(mux, vm.RouteRemoveNetwork, f.removeNetwork)
	f.handle(mux, vm.RouteCreateVM, f.createVM)
	f.handle(mux, vm.RouteVMs, f.listVMs)
	f.handle(mux, vm.RouteVM, f.getVM)
	f.handle(mux, vm.RouteStartVM, f.change(start))
	f.handle(mux, vm.RouteStopVM, f.change(stop))
	f.handle(mux, vm.RouteRestartVM, f.change(restart))
	f.handle(mux, vm.RouteKillVM, f.change(kill))
	f.handle(mux, vm.RouteDeleteVM, f.deleteVM)
	f.handle(mux, vm.RouteVMLogs, f.vmLogs)
	f.handle(mux, vm.RouteVMStats, f.vmStats)
	f.handle(mux, vm.RouteExec, f.exec)
	f.handle(mux, vm.RouteEndExec, f.endExec)
	f.handle(mux, vm.RouteDial, f.dial)

	// short on purpose: a socket's path is at most 104 bytes on a Mac.
	dir, err := os.MkdirTemp("", "vmhost")
	require.NoError(t, err)

	socket := filepath.Join(dir, "vmhost.sock")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	f.dir = dir
	f.endpoint = "unix://" + socket
	f.server = httptest.NewUnstartedServer(mux)
	_ = f.server.Listener.Close()
	f.server.Listener = listener
	f.server.Start()

	t.Cleanup(f.close)

	return f
}

// close takes vmhost away: what it holds is gone, and its socket with it.
func (f *fakeVMHost) close() {
	f.lock.Lock()
	hijacked := f.hijacked
	f.hijacked = nil

	for id, live := range f.live {
		close(live)
		delete(f.live, id)
	}
	f.lock.Unlock()

	for _, conn := range hijacked {
		_ = conn.Close()
	}

	f.server.CloseClientConnections()
	f.server.Close()

	_ = os.RemoveAll(f.dir)
}

// handle serves one route: written down, and refused when the test says so.
func (f *fakeVMHost) handle(mux *http.ServeMux, route string, serve http.HandlerFunc) {
	mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		f.lock.Lock()
		f.asked = append(f.asked, r.Method+" "+r.URL.RequestURI())
		refused := f.refusing[route]
		f.lock.Unlock()

		if refused != nil {
			f.refuse(w, refused)

			return
		}

		serve(w, r)
	})
}

// refuse answers an error as vmhost does.
func (f *fakeVMHost) refuse(w http.ResponseWriter, err error) {
	status, answer := vm.Describe(err)

	f.answer(w, status, answer)
}

func (f *fakeVMHost) answer(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// listing answers a list the way the test says vmhost answers one.
func (f *fakeVMHost) listing(w http.ResponseWriter, items any) {
	f.lock.Lock()
	wrapped := f.wrapped
	f.lock.Unlock()

	if wrapped {
		f.answer(w, http.StatusOK, map[string]any{"items": items})

		return
	}

	f.answer(w, http.StatusOK, items)
}

// requests is every request made so far.
func (f *fakeVMHost) requests() []string {
	f.lock.Lock()
	defer f.lock.Unlock()

	return slices.Clone(f.asked)
}

// refuseRoute makes a route answer err from now on.
func (f *fakeVMHost) refuseRoute(route string, err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.refusing[route] = err
}

// hold puts a VM in vmhost, as if it had been made there.
func (f *fakeVMHost) hold(v vm.VM) vm.VM {
	f.lock.Lock()
	defer f.lock.Unlock()

	if len(v.ID) == 0 {
		f.made++
		v.ID = fmt.Sprintf("%016x", 0xf000+f.made)
	}

	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}

	f.vms = append(f.vms, v)

	return v
}

// held is one VM as vmhost holds it now.
func (f *fakeVMHost) held(id string) (vm.VM, bool) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if i := f.index(id); i >= 0 {
		return f.vms[i], true
	}

	return vm.VM{}, false
}

func (f *fakeVMHost) index(id string) int {
	return slices.IndexFunc(f.vms, func(v vm.VM) bool { return v.ID == id })
}

// say is a line a VM writes: kept, and carried to whoever follows its output.
func (f *fakeVMHost) say(id string, line vm.LogLine) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.logs[id] = append(f.logs[id], line)

	if live, found := f.live[id]; found {
		live <- line
	}
}

// following reports whether somebody follows a VM's output right now.
func (f *fakeVMHost) following(id string) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	_, found := f.live[id]

	return found
}

// quiet ends the output of a VM somebody follows, as vmhost does once a VM's
// task has ended.
func (f *fakeVMHost) quiet(id string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if live, found := f.live[id]; found {
		close(live)
		delete(f.live, id)
	}
}

func (f *fakeVMHost) getInfo(w http.ResponseWriter, r *http.Request) {
	f.lock.Lock()
	info := f.info
	f.lock.Unlock()

	f.answer(w, http.StatusOK, info)
}

func (f *fakeVMHost) prepareImage(w http.ResponseWriter, r *http.Request) {
	var asked vm.PrepareImage
	if err := json.NewDecoder(r.Body).Decode(&asked); err != nil || len(asked.Image) == 0 {
		f.refuse(w, fmt.Errorf("%w: no image named", vm.ErrInvalid))

		return
	}

	sum := sha256.Sum256([]byte(asked.Image))
	digest := hex.EncodeToString(sum[:])

	prepared := vm.Image{
		Reference: asked.Image,
		Digest:    "sha256:" + digest,
		Root:      "/var/lib/workload-vmhost/images/" + digest[:12] + "/root.squashfs",
		Size:      1 << 20,
		Config:    vm.ImageConfig{Cmd: []string{"sh"}},
	}

	f.lock.Lock()
	f.images = slices.DeleteFunc(f.images, func(i vm.Image) bool { return i.Digest == prepared.Digest })
	f.images = append(f.images, prepared)
	f.lock.Unlock()

	f.answer(w, http.StatusOK, prepared)
}

func (f *fakeVMHost) listImages(w http.ResponseWriter, r *http.Request) {
	f.lock.Lock()
	images := slices.Clone(f.images)
	f.lock.Unlock()

	if images == nil {
		images = []vm.Image{}
	}

	f.listing(w, images)
}

func (f *fakeVMHost) deleteImage(w http.ResponseWriter, r *http.Request) {
	digest := r.PathValue("digest")

	f.lock.Lock()
	before := len(f.images)
	f.images = slices.DeleteFunc(f.images, func(i vm.Image) bool { return i.Digest == digest })
	removed := len(f.images) < before
	f.lock.Unlock()

	if !removed {
		f.refuse(w, fmt.Errorf("%w: image %s", vm.ErrNotFound, digest))

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeVMHost) ensureNetwork(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var spec vm.NetworkSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil || !vm.IsNetworkName(name) {
		f.refuse(w, fmt.Errorf("%w: network %q", vm.ErrInvalid, name))

		return
	}

	f.lock.Lock()
	made, found := f.networks[name]
	if !found {
		made = vm.Network{
			Name:       name,
			Subnet:     fmt.Sprintf("10.250.%d.0/24", len(f.networks)),
			Gateway:    fmt.Sprintf("10.250.%d.1", len(f.networks)),
			Masquerade: spec.Masquerade,
		}
		f.networks[name] = made
	}
	f.lock.Unlock()

	if made.Masquerade != spec.Masquerade {
		f.refuse(w, fmt.Errorf("%w: network %s routes out: %t", vm.ErrConflict, name, made.Masquerade))

		return
	}

	f.answer(w, http.StatusOK, made)
}

func (f *fakeVMHost) removeNetwork(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	f.lock.Lock()
	_, found := f.networks[name]
	inUse := f.inUse[name] > 0

	switch {
	case inUse:
		f.inUse[name]--
	case found:
		delete(f.networks, name)
	}
	f.lock.Unlock()

	switch {
	case inUse:
		f.refuse(w, fmt.Errorf("%w: %s", vm.ErrNetworkInUse, name))
	case !found:
		f.refuse(w, fmt.Errorf("%w: network %s", vm.ErrNotFound, name))
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (f *fakeVMHost) createVM(w http.ResponseWriter, r *http.Request) {
	var spec vm.Spec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		f.refuse(w, fmt.Errorf("%w: %v", vm.ErrInvalid, err))

		return
	}

	f.lock.Lock()

	if slices.ContainsFunc(f.vms, func(v vm.VM) bool { return v.Spec.Name == spec.Name }) {
		f.lock.Unlock()
		f.refuse(w, fmt.Errorf("%w: a vm is called %s already", vm.ErrConflict, spec.Name))

		return
	}

	if f.room > 0 && spec.Resources.Memory > f.room {
		f.lock.Unlock()
		f.refuse(w, fmt.Errorf("%w: %d bytes asked for, %d left", vm.ErrCapacity, spec.Resources.Memory, f.room))

		return
	}

	if f.room > 0 {
		f.room -= spec.Resources.Memory
	}

	f.made++
	made := vm.VM{
		ID:        fmt.Sprintf("%016x", f.made),
		Spec:      spec,
		State:     vm.StateCreated,
		CreatedAt: time.Now().UTC(),
	}

	f.vms = append(f.vms, made)
	f.lock.Unlock()

	f.answer(w, http.StatusCreated, vm.Created{ID: made.ID})
}

func (f *fakeVMHost) listVMs(w http.ResponseWriter, r *http.Request) {
	filters := r.URL.Query()[vm.QueryLabel]

	f.lock.Lock()
	matched := make([]vm.VM, 0, len(f.vms))
	for _, v := range f.vms {
		if f.unfiltered || v.Matches(filters) {
			matched = append(matched, v)
		}
	}
	f.lock.Unlock()

	f.listing(w, matched)
}

func (f *fakeVMHost) getVM(w http.ResponseWriter, r *http.Request) {
	found, ok := f.held(r.PathValue("id"))
	if !ok {
		f.refuse(w, fmt.Errorf("%w: vm %s", vm.ErrNotFound, r.PathValue("id")))

		return
	}

	f.answer(w, http.StatusOK, found)
}

// what a VM's life does to it, in this fake: enough to tell each apart.
func start(v *vm.VM, _ *http.Request) {
	v.State, v.StartedAt, v.Stopped = vm.StateRunning, time.Now().UTC(), false
	v.Generation++
	v.Interfaces = nil

	for i, attachment := range v.Spec.Networks {
		v.Interfaces = append(v.Interfaces, vm.Interface{
			Network: attachment.Network,
			Device:  fmt.Sprintf("wkt%s%d", v.ID[:8], i),
			MAC:     fmt.Sprintf("06:00:0a:fa:00:%02x", i+2),
			Address: fmt.Sprintf("10.250.%d.2/24", i),
			Aliases: attachment.Aliases,
		})
	}
}

func stop(v *vm.VM, r *http.Request) {
	v.State, v.ExitCode, v.Stopped, v.FinishedAt, v.Interfaces = vm.StateExited, 143, true, time.Now().UTC(), nil
	v.Reason = "stopped within " + r.URL.Query().Get(vm.QueryTimeout)
}

func restart(v *vm.VM, r *http.Request) {
	start(v, r)
	v.RestartCount++
}

func kill(v *vm.VM, _ *http.Request) {
	v.State, v.ExitCode, v.Stopped, v.FinishedAt, v.Interfaces = vm.StateExited, 137, true, time.Now().UTC(), nil
}

// change serves one thing done to a VM.
func (f *fakeVMHost) change(do func(*vm.VM, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		f.lock.Lock()
		i := f.index(id)
		if i >= 0 {
			do(&f.vms[i], r)
		}
		f.lock.Unlock()

		if i < 0 {
			f.refuse(w, fmt.Errorf("%w: vm %s", vm.ErrNotFound, id))

			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (f *fakeVMHost) deleteVM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	f.lock.Lock()
	i := f.index(id)
	if i >= 0 {
		f.vms = slices.Delete(f.vms, i, i+1)
	}
	f.lock.Unlock()

	if i < 0 {
		f.refuse(w, fmt.Errorf("%w: vm %s", vm.ErrNotFound, id))

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// running is nil for a VM that runs, and what vmhost says otherwise.
func (f *fakeVMHost) running(id string) error {
	found, ok := f.held(id)

	switch {
	case !ok:
		return fmt.Errorf("%w: vm %s", vm.ErrNotFound, id)
	case found.State != vm.StateRunning:
		return fmt.Errorf("%w: vm %s is %s", vm.ErrNotRunning, id, found.State)
	default:
		return nil
	}
}

func (f *fakeVMHost) vmLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, ok := f.held(id); !ok {
		f.refuse(w, fmt.Errorf("%w: vm %s", vm.ErrNotFound, id))

		return
	}

	query := r.URL.Query()

	after, _ := strconv.ParseUint(query.Get(vm.QueryAfter), 10, 64)

	var since time.Time
	if asked := query.Get(vm.QuerySince); len(asked) > 0 {
		parsed, err := time.Parse(time.RFC3339Nano, asked)
		if err != nil {
			f.refuse(w, fmt.Errorf("%w: since %q", vm.ErrInvalid, asked))

			return
		}

		since = parsed
	}

	follow := query.Get(vm.QueryFollow) == "1"

	// what was written so far, and what is written from now on, taken
	// together: a line is in one or the other, never both.
	f.lock.Lock()
	written := slices.Clone(f.logs[id])

	var live chan vm.LogLine
	if follow {
		live = make(chan vm.LogLine, 16)
		f.live[id] = live
	}
	f.lock.Unlock()

	if follow {
		defer func() {
			f.lock.Lock()
			defer f.lock.Unlock()

			if f.live[id] == live {
				delete(f.live, id)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	flusher := http.NewResponseController(w)

	send := func(line vm.LogLine) bool {
		if line.Seq <= after || line.At.Before(since) {
			return true
		}

		return encoder.Encode(line) == nil && flusher.Flush() == nil
	}

	for _, line := range written {
		if !send(line) {
			return
		}
	}

	if !follow {
		return
	}

	_ = flusher.Flush()

	for {
		select {
		case line, open := <-live:
			if !open || !send(line) {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (f *fakeVMHost) vmStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := f.running(id); err != nil {
		f.refuse(w, err)

		return
	}

	f.lock.Lock()
	used := f.stats[id]
	refused := f.statsRefused[id]
	f.lock.Unlock()

	if refused != nil {
		f.refuse(w, refused)

		return
	}

	f.answer(w, http.StatusOK, used)
}

// switchProtocols takes the connection a request came on over and agrees to
// what it asked to become, with the headers given.
func (f *fakeVMHost) switchProtocols(w http.ResponseWriter, protocol string, headers ...string) (net.Conn, *bufio.ReadWriter) {
	conn, buffered, err := http.NewResponseController(w).Hijack()
	if err != nil {
		f.t.Errorf("vmhost could not take a connection over: %v", err)

		return nil, nil
	}

	f.lock.Lock()
	f.hijacked = append(f.hijacked, conn)
	f.lock.Unlock()

	_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: %s\r\n", protocol)
	for _, header := range headers {
		_, _ = fmt.Fprintf(buffered, "%s\r\n", header)
	}

	_, _ = fmt.Fprint(buffered, "\r\n")

	if err := buffered.Flush(); err != nil {
		_ = conn.Close()

		return nil, nil
	}

	return conn, buffered
}

// exec runs a command that says back what it is given, says what size its
// terminal is when told, and ends with 3 when it is given "exit".
func (f *fakeVMHost) exec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := f.running(id); err != nil {
		f.refuse(w, err)

		return
	}

	var exec guest.Exec
	if err := json.NewDecoder(r.Body).Decode(&exec); err != nil {
		f.refuse(w, fmt.Errorf("%w: %v", vm.ErrInvalid, err))

		return
	}

	if !strings.EqualFold(r.Header.Get("Upgrade"), vm.UpgradeExec) {
		f.refuse(w, fmt.Errorf("%w: a command runs on a connection upgraded to %s", vm.ErrInvalid, vm.UpgradeExec))

		return
	}

	f.lock.Lock()
	f.execs = append(f.execs, exec)
	f.lock.Unlock()

	conn, buffered := f.switchProtocols(w, vm.UpgradeExec, vm.ExecIDHeader+": e1")
	if conn == nil {
		return
	}
	defer conn.Close()

	for {
		frame, err := guest.ReadFrame(buffered)
		if err != nil {
			return
		}

		switch frame.Type {
		case guest.FrameStdin:
			if strings.TrimSpace(string(frame.Payload)) == "exit" {
				_ = guest.WriteFrame(conn, guest.FrameExit, guest.ExitPayload(3))

				return
			}

			_ = guest.WriteFrame(conn, guest.FrameStdout, frame.Payload)
		case guest.FrameResize:
			rows, cols, err := guest.ParseResize(frame.Payload)
			if err != nil {
				return
			}

			size := fmt.Sprintf("%dx%d", rows, cols)

			f.lock.Lock()
			f.resizes = append(f.resizes, size)
			f.lock.Unlock()

			_ = guest.WriteFrame(conn, guest.FrameStderr, []byte(size))
		}
	}
}

func (f *fakeVMHost) endExec(w http.ResponseWriter, r *http.Request) {
	var end guest.EndExec
	if err := json.NewDecoder(r.Body).Decode(&end); err != nil {
		f.refuse(w, fmt.Errorf("%w: %v", vm.ErrInvalid, err))

		return
	}

	f.lock.Lock()
	f.ends[r.PathValue("id")+"/"+r.PathValue("exec")] = end
	f.lock.Unlock()

	f.answer(w, http.StatusOK, guest.Ended{Signalled: true})
}

// dial carries a connection to a VM's port to dialTo.
func (f *fakeVMHost) dial(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := f.running(id); err != nil {
		f.refuse(w, err)

		return
	}

	if p, err := strconv.ParseUint(r.URL.Query().Get(vm.QueryPort), 10, 16); err != nil || p == 0 {
		f.refuse(w, fmt.Errorf("%w: port %q", vm.ErrInvalid, r.URL.Query().Get(vm.QueryPort)))

		return
	}

	f.lock.Lock()
	target := f.dialTo
	f.lock.Unlock()

	upstream, err := net.Dial("tcp", target)
	if err != nil {
		f.refuse(w, errors.New("nothing answers on that port"))

		return
	}
	defer upstream.Close()

	conn, buffered := f.switchProtocols(w, vm.UpgradeDial)
	if conn == nil {
		return
	}
	defer conn.Close()

	go func() {
		_, _ = io.Copy(upstream, buffered)
		_ = upstream.(*net.TCPConn).CloseWrite()
	}()

	_, _ = io.Copy(conn, upstream)
}
