package registry

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	driverMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/driver"
)

const node = "workload-orchestrator-01"

func discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// configured is the specs WORKLOAD_ORCHESTRATOR_RUNTIMES gives.
func configured(t *testing.T, value string) []driver.Spec {
	t.Helper()

	specs, err := driver.ParseSpecs(value)
	require.NoError(t, err)

	return specs
}

// fake is a driver of a class and kind, which offers it healthy and lets go
// when it is closed.
func fake(class runtime.Class, kind driver.Kind) *driverMock.MockDriver {
	d := &driverMock.MockDriver{}
	d.On("Class").Return(class).Maybe()
	d.On("Kind").Return(kind).Maybe()
	d.On("Offer", mock.Anything).Return(runtime.Offer{Class: class, Healthy: true}).Maybe()
	d.On("Close").Return(nil).Maybe()

	return d
}

// factoryOf is a factory that hands out the given drivers by class, and
// remembers what it was asked for.
type factoryOf struct {
	drivers map[runtime.Class]*driverMock.MockDriver
	asked   []driver.Spec
	nodes   []string
}

func (f *factoryOf) build(ctx context.Context, nodeName string, spec driver.Spec) (driver.Driver, error) {
	f.asked = append(f.asked, spec)
	f.nodes = append(f.nodes, nodeName)

	d, ok := f.drivers[spec.Class]
	if !ok {
		return nil, errors.New("no driver for " + string(spec.Class))
	}

	return d, nil
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("a driver for every class, by the factory of its kind, in the order configured", func(t *testing.T) {
		t.Parallel()

		sysbox := fake(runtime.Sysbox, driver.KindContainer)
		firecracker := fake(runtime.Firecracker, driver.KindMicroVM)

		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: sysbox}}
		microvms := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Firecracker: firecracker}}

		r, err := New(context.Background(), node,
			configured(t, "sysbox=container@tcp://docker:2375?oci-runtime=sysbox-runc,firecracker=microvm@unix:///run/workload-vmhost/vmhost.sock"),
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build, driver.KindMicroVM: microvms.build},
			discard(),
		)
		require.NoError(t, err)

		assert.Equal(t, []driver.Driver{sysbox, firecracker}, r.All())

		// each factory is told which node it builds for, and given its spec whole.
		assert.Equal(t, []string{node}, containers.nodes)
		assert.Equal(t, url.Values{driver.OptionOCIRuntime: {"sysbox-runc"}}, containers.asked[0].Options)
		assert.Equal(t, "unix:///run/workload-vmhost/vmhost.sock", microvms.asked[0].Endpoint)

		found, err := r.For(runtime.Firecracker)
		require.NoError(t, err)
		assert.Same(t, firecracker, found)
	})

	t.Run("a class naming none is sysbox", func(t *testing.T) {
		t.Parallel()

		sysbox := fake(runtime.Sysbox, driver.KindContainer)
		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: sysbox}}

		r, err := New(context.Background(), node, configured(t, "sysbox=container@tcp://docker:2375"),
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build}, discard())
		require.NoError(t, err)

		found, err := r.For("")
		require.NoError(t, err)
		assert.Same(t, sysbox, found)
	})

	t.Run("a class this node does not offer is one it does not know", func(t *testing.T) {
		t.Parallel()

		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: fake(runtime.Sysbox, driver.KindContainer)}}

		r, err := New(context.Background(), node, configured(t, "sysbox=container@tcp://docker:2375"),
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build}, discard())
		require.NoError(t, err)

		_, err = r.For(runtime.Firecracker)
		assert.ErrorIs(t, err, driver.ErrUnknownClass)
	})

	t.Run("a class that cannot run tasks yet is built all the same", func(t *testing.T) {
		t.Parallel()

		sysbox := &driverMock.MockDriver{}
		sysbox.On("Class").Return(runtime.Sysbox)
		sysbox.On("Kind").Return(driver.KindContainer)
		sysbox.On("Offer", mock.Anything).Return(runtime.Offer{Class: runtime.Sysbox, Reason: "docker cannot be reached"}).Once()
		defer sysbox.AssertExpectations(t)

		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: sysbox}}

		r, err := New(context.Background(), node, configured(t, "sysbox=container@tcp://docker:2375"),
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build}, discard())
		require.NoError(t, err)

		assert.Len(t, r.All(), 1)
	})

	t.Run("every driver is let go of together", func(t *testing.T) {
		t.Parallel()

		sysbox := fake(runtime.Sysbox, driver.KindContainer)
		gvisor := fake("gvisor", driver.KindContainer)

		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: sysbox, "gvisor": gvisor}}

		r, err := New(context.Background(), node, configured(t, "sysbox=container@tcp://docker:2375,gvisor=container@tcp://docker:2375?oci-runtime=runsc&network-prefix=gvisor-"),
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build}, discard())
		require.NoError(t, err)

		require.NoError(t, r.Close())

		sysbox.AssertCalled(t, "Close")
		gvisor.AssertCalled(t, "Close")
	})
}

func TestNew_refused(t *testing.T) {
	t.Parallel()

	t.Run("a kind nobody registered a factory for", func(t *testing.T) {
		t.Parallel()

		_, err := New(context.Background(), node, configured(t, "kata=kata@unix:///run/kata.sock"),
			map[driver.Kind]driver.Factory{driver.KindContainer: (&factoryOf{}).build}, discard())

		require.Error(t, err)
		assert.Contains(t, err.Error(), `"kata" kind`)
	})

	t.Run("a class configured twice", func(t *testing.T) {
		t.Parallel()

		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: fake(runtime.Sysbox, driver.KindContainer)}}

		specs := []driver.Spec{
			{Class: runtime.Sysbox, Kind: driver.KindContainer, Endpoint: "tcp://docker:2375"},
			{Class: runtime.Sysbox, Kind: driver.KindContainer, Endpoint: "tcp://other:2375"},
		}

		_, err := New(context.Background(), node, specs,
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build}, discard())

		assert.Error(t, err)
	})

	t.Run("a spec its factory refuses, and nothing already built is left behind", func(t *testing.T) {
		t.Parallel()

		sysbox := fake(runtime.Sysbox, driver.KindContainer)
		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: sysbox}}

		_, err := New(context.Background(), node, configured(t, "sysbox=container@tcp://docker:2375,gvisor=container@tcp://docker:2375"),
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build}, discard())

		require.Error(t, err)
		assert.Contains(t, err.Error(), `"gvisor"`)

		sysbox.AssertCalled(t, "Close")
	})

	t.Run("a factory that builds another class than it was asked for", func(t *testing.T) {
		t.Parallel()

		wrong := fake("gvisor", driver.KindContainer)
		containers := &factoryOf{drivers: map[runtime.Class]*driverMock.MockDriver{runtime.Sysbox: wrong}}

		_, err := New(context.Background(), node, configured(t, "sysbox=container@tcp://docker:2375"),
			map[driver.Kind]driver.Factory{driver.KindContainer: containers.build}, discard())

		assert.Error(t, err)
		wrong.AssertCalled(t, "Close")
	})

	t.Run("nothing configured at all", func(t *testing.T) {
		t.Parallel()

		_, err := New(context.Background(), node, nil, map[driver.Kind]driver.Factory{}, discard())
		assert.Error(t, err)
	})
}
