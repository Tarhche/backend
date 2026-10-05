//go:build microsandbox && integration

package microsandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// The integration tests run the engine for real, where it runs: in the
// vmhost's image, on a host with /dev/kvm, the way scripts/vmhost-integration.sh
// runs them. They share one home and one engine and run one after another;
// each removes what it made, and TestShutdown, which stops everything, is the
// last of them.
//
// The test process plays the orchestrator: the only address a VM's published
// ports take connections from is the container's own, which the test dials
// from, and 127.0.0.1 is the address of anybody else.

const (
	itFirstPort port.Port = 21000
	itLastPort  port.Port = 21999

	// itOwner is whose every VM of the tests is.
	itOwner = "it-owner"

	// elsewhere is an address of the container that is not the
	// orchestrator's.
	elsewhere = "127.0.0.1"
)

var (
	shared        Engine
	sharedFailure error
	sharing       sync.Once
)

// engineFor is the engine the tests share, made by the first test that asks.
// What an earlier run left behind is removed first.
func engineFor(t *testing.T) Engine {
	t.Helper()

	sharing.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		shared, sharedFailure = New(ctx, itOptions(t))
		if sharedFailure != nil {
			return
		}

		instances, err := shared.List(ctx)
		if err != nil {
			sharedFailure = err

			return
		}

		for _, instance := range instances {
			if strings.HasPrefix(instance.ID, "it-") || len(instance.Labels[vm.LabelTask]) > 0 {
				_ = shared.Delete(ctx, instance.ID)
			}
		}
	})

	require.NoError(t, sharedFailure, "the engine could not be made")

	return shared
}

// itOptions are what every engine of the tests is given.
func itOptions(t testing.TB) Options {
	bind := os.Getenv("VMHOST_IT_BIND")
	if len(bind) == 0 {
		bind = ownAddress(t)
	}

	return Options{
		Home:                envOr("MSB_HOME", "/data/msb"),
		BindAddress:         bind,
		OrchestratorAddress: bind,
		FirstPort:           itFirstPort,
		LastPort:            itLastPort,
		DockerImage:         envOr("VMHOST_IT_DOCKER_IMAGE", "docker:29-dind"),
		MaxConcurrentBoots:  2,
		Logger:              slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
}

func envOr(name string, fallback string) string {
	if value := os.Getenv(name); len(value) > 0 {
		return value
	}

	return fallback
}

// ownAddress is the container's own IPv4 address.
func ownAddress(t testing.TB) string {
	addresses, err := net.InterfaceAddrs()
	require.NoError(t, err)

	for _, address := range addresses {
		if ip, ok := address.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
			return ip.IP.String()
		}
	}

	t.Fatal("the container has no address of its own")

	return ""
}

func machineImage() string {
	return envOr("VMHOST_IT_MACHINE_IMAGE", "alpine:3.20")
}

// machine is a small machine VM: a persistent disk, its ingress and egress
// allowed, and labelled as a user's VM.
func machine(id string, changes ...func(spec *vm.Spec)) vm.Spec {
	spec := vm.Spec{
		ID:             id,
		Kind:           vm.KindMachine,
		Image:          machineImage(),
		Resources:      vm.Resources{CPUs: 1, Memory: 256 << 20, Disk: 1 << 30},
		Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
		PersistentDisk: true,
		Labels: map[string]string{
			vm.LabelPurpose: vm.PurposeVM,
			vm.LabelVM:      id,
			vm.LabelOwner:   itOwner,
			vm.LabelSlug:    id + "-slug",
		},
	}

	for _, change := range changes {
		change(&spec)
	}

	return spec
}

// dockerVM is a Docker VM, sized the way the control plane's floor for one
// is.
func dockerVM(id string, changes ...func(spec *vm.Spec)) vm.Spec {
	return machine(id, append([]func(spec *vm.Spec){func(spec *vm.Spec) {
		spec.Kind = vm.KindDocker
		spec.Image = envOr("VMHOST_IT_DOCKER_IMAGE", "docker:29-dind")
		spec.Resources = vm.Resources{CPUs: 2, Memory: 1 << 30, Disk: 6 << 30}
	}}, changes...)...)
}

func withPorts(ports ...port.Port) func(spec *vm.Spec) {
	return func(spec *vm.Spec) {
		spec.Ports = ports
	}
}

func withNetwork(ingress vm.Access, egress vm.Access) func(spec *vm.Spec) {
	return func(spec *vm.Spec) {
		spec.Network = vm.Network{Ingress: ingress, Egress: egress}
	}
}

// created makes an instance, and removes it once the test is over.
func created(t *testing.T, e Engine, spec vm.Spec) vm.Instance {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()

	started := time.Now()

	instance, err := e.Create(ctx, spec)
	require.NoError(t, err, "creating %s", spec.ID)

	t.Logf("%s: created in %s", spec.ID, time.Since(started).Round(time.Millisecond))
	t.Cleanup(func() { removed(t, e, spec.ID) })

	return instance
}

// removed removes an instance, which one that is gone already is.
func removed(t *testing.T, e Engine, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	assert.NoError(t, e.Delete(ctx, id), "removing %s", id)
}

func inspected(t *testing.T, e Engine, id string) vm.Instance {
	t.Helper()

	instance, err := e.Inspect(t.Context(), id)
	require.NoError(t, err)

	return instance
}

// ran is how a command run in an instance went.
type ran struct {
	stdout string
	stderr string
	code   int
}

// run runs a shell script in an instance, with nothing on its input.
func run(t *testing.T, e Engine, id string, script string) ran {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	session, err := e.Exec(ctx, id, vm.ExecOptions{Command: []string{"/bin/sh", "-c", script}})
	require.NoError(t, err, "exec %q in %s", script, id)
	defer session.Close()

	_ = session.Stdin().Close()

	var stdout, stderr bytes.Buffer

	var reading sync.WaitGroup
	reading.Go(func() { _, _ = io.Copy(&stdout, session.Stdout()) })
	reading.Go(func() { _, _ = io.Copy(&stderr, session.Stderr()) })
	reading.Wait()

	code, err := session.Wait(ctx)
	require.NoError(t, err)

	return ran{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// runOK runs a shell script that has to succeed, and is what it printed.
func runOK(t *testing.T, e Engine, id string, script string) string {
	t.Helper()

	r := run(t, e, id, script)
	require.Equal(t, 0, r.code, "%q in %s: stdout %q, stderr %q", script, id, r.stdout, r.stderr)

	return strings.TrimSpace(r.stdout)
}

// serve answers HTTP on a guest port of an alpine VM with body, from a
// process the exec that starts it leaves running.
func serve(t *testing.T, e Engine, id string, guest port.Port, body string) {
	t.Helper()

	runOK(t, e, id, fmt.Sprintf(`mkdir -p /srv && cat > /srv/respond.sh <<'EOF'
#!/bin/sh
while read -r line; do line=$(printf '%%s' "$line" | tr -d '\r'); [ -z "$line" ] && break; done
printf 'HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n%s\n'
EOF
chmod +x /srv/respond.sh
(setsid nc -lk -p %d -e /srv/respond.sh </dev/null >/dev/null 2>&1 &)
sleep 0.3`, body, guest))
}

// endpoint is where an instance's guest port is reached.
func endpoint(t *testing.T, instance vm.Instance, guest port.Port) string {
	t.Helper()

	for _, endpoint := range instance.Endpoints {
		if endpoint.Port == guest {
			return endpoint.Address
		}
	}

	t.Fatalf("%s publishes no port %d: %v", instance.ID, guest, instance.Endpoints)

	return ""
}

// fetch gets / from address, connecting from the address from.
func fetch(address string, from string) (string, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(from)}}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, DisableKeepAlives: true},
	}

	response, err := client.Get("http://" + address + "/")
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)

	return strings.TrimSpace(string(body)), err
}

// eventually waits for condition, and fails the test when it does not hold
// in time.
func eventually(t *testing.T, timeout time.Duration, condition func() bool, format string, args ...any) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("not in %s: "+format, append([]any{timeout}, args...)...)
		}

		time.Sleep(250 * time.Millisecond)
	}
}

// archived writes an instance's snapshot to a file of the test's, and hands
// it back from its start.
func archived(t *testing.T, e Engine, id string) (*os.File, vm.Archive) {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "archive-*.msb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	started := time.Now()

	archive, err := e.Snapshot(t.Context(), id, file)
	require.NoError(t, err, "snapshotting %s", id)

	info, err := file.Stat()
	require.NoError(t, err)

	t.Logf("%s: snapshot of %d bytes in %s", id, info.Size(), time.Since(started).Round(time.Millisecond))

	assert.Equal(t, archiveEngine(), archive.Engine)
	assert.Equal(t, info.Size(), archive.Size)

	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)

	return file, archive
}

// restored restores a new instance from an archive file, and removes it once
// the test is over.
func restored(t *testing.T, e Engine, spec vm.Spec, archive *os.File) vm.Instance {
	t.Helper()

	instance := restoredOnto(t, e, spec, archive)
	t.Cleanup(func() { removed(t, e, spec.ID) })

	return instance
}

// restoredOnto restores an instance that is there already from an archive
// file, which leaves removing it to whoever made it.
func restoredOnto(t *testing.T, e Engine, spec vm.Spec, archive *os.File) vm.Instance {
	t.Helper()

	_, err := archive.Seek(0, io.SeekStart)
	require.NoError(t, err)

	started := time.Now()

	instance, err := e.Restore(t.Context(), spec, archive)
	require.NoError(t, err, "restoring %s", spec.ID)

	t.Logf("%s: restored in %s", spec.ID, time.Since(started).Round(time.Millisecond))

	return instance
}
