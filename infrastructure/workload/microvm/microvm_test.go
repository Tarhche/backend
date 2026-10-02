package microvm

import (
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestNew(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	spec := func(change func(*driver.Spec)) driver.Spec {
		s := driver.Spec{Class: runtime.Firecracker, Kind: driver.KindMicroVM, Endpoint: "unix:///tmp/vmhost.sock", Options: url.Values{}}
		change(&s)

		return s
	}

	t.Run("configuration that is wrong stops the orchestrator from starting", func(t *testing.T) {
		t.Parallel()

		for name, wrong := range map[string]driver.Spec{
			"another kind":           spec(func(s *driver.Spec) { s.Kind = driver.KindContainer }),
			"no class":               spec(func(s *driver.Spec) { s.Class = "" }),
			"an endpoint not a unix": spec(func(s *driver.Spec) { s.Endpoint = "tcp://vmhost:2375" }),
			"a container's option":   spec(func(s *driver.Spec) { s.Options.Set(driver.OptionAdvertiseHost, "docker") }),
			"an option misspelled":   spec(func(s *driver.Spec) { s.Options.Set("node-mem", "1") }),
			"a share of vmhost":      spec(func(s *driver.Spec) { s.Options.Set(driver.OptionNodeMemory, "8589934592") }),
		} {
			_, err := New(t.Context(), orchestrator, wrong, logger)
			assert.Error(t, err, name)
		}

		_, err := New(t.Context(), "", spec(func(*driver.Spec) {}), logger)
		assert.Error(t, err, "a driver labels its vms with its node's name, so it needs one")
	})

	t.Run("a vmhost that does not answer does not stop the orchestrator from starting", func(t *testing.T) {
		t.Parallel()

		d, err := New(t.Context(), orchestrator, spec(func(s *driver.Spec) {
			s.Endpoint = "unix://" + filepath.Join(os.TempDir(), "no-vmhost-here.sock")
		}), logger)
		require.NoError(t, err)
		defer d.Close()

		offer := d.Offer(t.Context())

		assert.False(t, offer.Healthy)
		assert.Contains(t, offer.Reason, "cannot be reached")
	})

	t.Run("a spec that names no endpoint reaches vmhost where it listens by default", func(t *testing.T) {
		t.Parallel()

		d, err := New(t.Context(), orchestrator, spec(func(s *driver.Spec) { s.Endpoint = "" }), logger)
		require.NoError(t, err)
		defer d.Close()

		assert.Equal(t, vm.DefaultSocket, d.client.socketPath)
	})
}

func TestNewFactory(t *testing.T) {
	f := newFakeVMHost(t)

	built, err := NewFactory(slog.New(slog.DiscardHandler))(t.Context(), orchestrator, driver.Spec{
		Class:    runtime.Firecracker,
		Kind:     driver.KindMicroVM,
		Endpoint: f.endpoint,
		Options:  url.Values{},
	})
	require.NoError(t, err)
	defer built.Close()

	assert.Equal(t, runtime.Firecracker, built.Class())
	assert.Equal(t, driver.KindMicroVM, built.Kind())
	assert.NotNil(t, built.Tasks())
	assert.NotNil(t, built.Networks())
	assert.NotNil(t, built.Node())
}

func TestDriver_Offer(t *testing.T) {
	t.Run("the class is offered as vmhost says it runs it", func(t *testing.T) {
		f := newFakeVMHost(t)

		offer := newDriver(t, f).Offer(t.Context())

		assert.Equal(t, runtime.Offer{
			Class:        runtime.Firecracker,
			Driver:       "microvm",
			Version:      "1.2.3",
			Healthy:      true,
			Capabilities: f.info.Capabilities,
			Capacity:     f.info.Capacity,
		}, offer)
	})

	t.Run("an offer is answered from what vmhost last said, without asking it again", func(t *testing.T) {
		f := newFakeVMHost(t)
		d := newDriver(t, f)

		before := len(f.requests())

		for range 10 {
			d.Offer(t.Context())
		}

		assert.LessOrEqual(t, len(f.requests())-before, 1, "at most the one asking the driver does every second")
	})

	t.Run("a vmhost that cannot run vms right now is offered unhealthy, with its reason", func(t *testing.T) {
		f := newFakeVMHost(t)
		d := newDriver(t, f)

		f.lock.Lock()
		f.info.Healthy, f.info.Reason = false, "/dev/kvm is not there"
		f.lock.Unlock()

		d.refresh(t.Context())

		offer := d.Offer(t.Context())

		assert.False(t, offer.Healthy)
		assert.Equal(t, "/dev/kvm is not there", offer.Reason)
	})

	t.Run("a vmhost that went away is offered unhealthy, and what it could do is still said", func(t *testing.T) {
		f := newFakeVMHost(t)
		d := newDriver(t, f)

		f.close()
		d.refresh(t.Context())

		offer := d.Offer(t.Context())

		assert.False(t, offer.Healthy)
		assert.Contains(t, offer.Reason, vm.ErrUnavailable.Error())
		assert.Equal(t, f.info.Capabilities, offer.Capabilities)
	})

	t.Run("a vmhost that has not answered for a while is offered unhealthy", func(t *testing.T) {
		f := newFakeVMHost(t)
		d := newDriver(t, f)

		d.lock.Lock()
		d.answered = time.Now().Add(-time.Minute)
		d.lock.Unlock()

		offer := d.Offer(t.Context())

		assert.False(t, offer.Healthy)
		assert.Contains(t, offer.Reason, "has not answered since")
	})

	t.Run("what vmhost says is kept fresh in the background", func(t *testing.T) {
		f := newFakeVMHost(t)
		d := newDriver(t, f)

		f.lock.Lock()
		f.info.Healthy, f.info.Reason = false, "the fabric cannot be set up"
		f.lock.Unlock()

		require.Eventually(t, func() bool {
			return d.Offer(t.Context()).Reason == "the fabric cannot be set up"
		}, 5*time.Second, 50*time.Millisecond)
	})
}

func TestDriver_Close(t *testing.T) {
	f := newFakeVMHost(t)

	held := ours(f, "u-1", "nginx-xkfqz")

	d := newDriver(t, f)

	require.NoError(t, d.Close())
	require.NoError(t, d.Close(), "closing twice is closing once")

	stillThere, found := f.held(held.ID)
	require.True(t, found, "what a driver runs is vmhost's, and carries on")
	assert.Equal(t, vm.StateRunning, stillThere.State)

	// what asks vmhost on its own, once a second, has stopped asking.
	select {
	case <-d.stopped:
	default:
		t.Fatal("a closed driver still watches vmhost")
	}
}
