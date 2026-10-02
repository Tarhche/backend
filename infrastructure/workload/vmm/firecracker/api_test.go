package firecracker

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// apiCall is one request a fake firecracker was sent.
type apiCall struct {
	Method string
	Path   string
	Body   string
}

// fakeAPI stands in for a machine's firecracker API: it takes every
// configuration it is sent, refuses what it is told to, and remembers what it
// was sent.
type fakeAPI struct {
	lock    sync.Mutex
	calls   []apiCall
	refuses map[string]string
}

func (f *fakeAPI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	f.lock.Lock()
	f.calls = append(f.calls, apiCall{Method: r.Method, Path: r.URL.Path, Body: strings.TrimSpace(string(body))})
	fault, refused := f.refuses[r.URL.Path]
	f.lock.Unlock()

	if refused {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(rw).Encode(models.Error{FaultMessage: fault})

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (f *fakeAPI) sent() []apiCall {
	f.lock.Lock()
	defer f.lock.Unlock()

	return append([]apiCall(nil), f.calls...)
}

// serveAPI stands a fake firecracker API up on a unix socket of its own, which
// is somewhere short: a socket's path is at most 104 bytes on darwin.
func serveAPI(t *testing.T, api http.Handler) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "fc")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socket := filepath.Join(dir, "firecracker.socket")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := &http.Server{Handler: api}
	go func() { _ = server.Serve(listener) }()

	t.Cleanup(func() { _ = server.Close() })

	return socket
}

func TestPlanOf(t *testing.T) {
	t.Parallel()

	t.Run("a machine is told of its files by the names its own directory gives them", func(t *testing.T) {
		t.Parallel()

		s := spec()
		s.KernelArgs = guest.KernelArgs + " workload.debug"

		plan := planOf(s)

		vcpus, memory, smt := int64(2), int64(256), false
		assert.Equal(t, models.MachineConfiguration{VcpuCount: &vcpus, MemSizeMib: &memory, Smt: &smt}, plan.machine)

		kernel := "vmlinux"
		assert.Equal(t, models.BootSource{KernelImagePath: &kernel, InitrdPath: "initrd", BootArgs: guest.KernelArgs + " workload.debug"}, plan.boot)

		image, scratch, readOnly, writable, notRoot := "drive0", "drive1", true, false, false
		assert.Equal(t, []models.Drive{
			{DriveID: &image, PathOnHost: &image, IsReadOnly: &readOnly, IsRootDevice: &notRoot},
			{DriveID: &scratch, PathOnHost: &scratch, IsReadOnly: &writable, IsRootDevice: &notRoot},
		}, plan.drives, "the image first, then the scratch disk, neither of them the root: the agent puts the root together")

		nic, device := "eth0", "wkt0123456789ab"
		assert.Equal(t, []models.NetworkInterface{{IfaceID: &nic, HostDevName: &device, GuestMac: "02:fc:00:00:00:01"}}, plan.nics)

		cid, socket := int64(3), "run/v.sock"
		assert.Equal(t, models.Vsock{GuestCid: &cid, UdsPath: &socket}, plan.vsock)
	})

	t.Run("a machine asked for with no command line boots with the guest's", func(t *testing.T) {
		t.Parallel()

		s := spec()
		s.KernelArgs, s.Initrd, s.Drives, s.NICs = "", "", nil, nil

		plan := planOf(s)

		assert.Equal(t, guest.KernelArgs, plan.boot.BootArgs)
		assert.Empty(t, plan.boot.InitrdPath, "no initramfs is asked for")
		assert.Empty(t, plan.drives)
		assert.Empty(t, plan.nics, "a machine with no network has no network device at all")
	})
}

func TestConfigure(t *testing.T) {
	t.Parallel()

	t.Run("a machine is told what it is, in order, and then started", func(t *testing.T) {
		t.Parallel()

		api := &fakeAPI{}
		socket := serveAPI(t, api)

		require.NoError(t, configure(t.Context(), socket, planOf(spec())))

		assert.Equal(t, []apiCall{
			{Method: http.MethodPut, Path: "/machine-config", Body: `{"mem_size_mib":256,"smt":false,"vcpu_count":2}`},
			{Method: http.MethodPut, Path: "/boot-source", Body: `{"boot_args":"` + guest.KernelArgs + `","initrd_path":"initrd","kernel_image_path":"vmlinux"}`},
			{Method: http.MethodPut, Path: "/drives/drive0", Body: `{"drive_id":"drive0","is_read_only":true,"is_root_device":false,"path_on_host":"drive0"}`},
			{Method: http.MethodPut, Path: "/drives/drive1", Body: `{"drive_id":"drive1","is_read_only":false,"is_root_device":false,"path_on_host":"drive1"}`},
			{Method: http.MethodPut, Path: "/network-interfaces/eth0", Body: `{"guest_mac":"02:fc:00:00:00:01","host_dev_name":"wkt0123456789ab","iface_id":"eth0"}`},
			{Method: http.MethodPut, Path: "/vsock", Body: `{"guest_cid":3,"uds_path":"run/v.sock"}`},
			{Method: http.MethodPut, Path: "/actions", Body: `{"action_type":"InstanceStart"}`},
		}, api.sent())
	})

	t.Run("what firecracker refuses says what it was and why, and nothing after it is sent", func(t *testing.T) {
		t.Parallel()

		api := &fakeAPI{refuses: map[string]string{"/drives/drive1": "drive1: No such file or directory"}}
		socket := serveAPI(t, api)

		err := configure(t.Context(), socket, planOf(spec()))

		assert.ErrorContains(t, err, "the machine's disk 1 was refused")
		assert.ErrorContains(t, err, "No such file or directory")

		calls := api.sent()
		require.NotEmpty(t, calls)
		assert.Equal(t, "/drives/drive1", calls[len(calls)-1].Path, "the machine is never started")
	})

	t.Run("a firecracker that is not there is not configured", func(t *testing.T) {
		t.Parallel()

		err := configure(t.Context(), filepath.Join(t.TempDir(), "gone.socket"), planOf(spec()))

		assert.ErrorContains(t, err, "the machine's size was refused")
	})
}
