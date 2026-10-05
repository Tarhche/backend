//go:build !microsandbox

package vmhost

import (
	"log/slog"
	"testing"

	"github.com/danceable/container"
	"github.com/danceable/provider"
	"github.com/danceable/provider/adapters/danceable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/microsandbox"
)

// TestProvider_NotBuilt boots a vmhost in a build without the engine, which
// is every build but the vmhost image's: it refuses, and says why.
func TestProvider_NotBuilt(t *testing.T) {
	t.Parallel()

	c := danceable.New(container.New())

	vmhostSettings := settings()
	vmhostSettings.Memory = 4 << 30

	require.NoError(t, c.Bind(func() *configs.WorkloadVMHost { return vmhostSettings }, provider.Singleton()))
	require.NoError(t, c.Bind(func(name string) *slog.Logger { return slog.New(slog.DiscardHandler) }, provider.Lazy()))

	p := NewProvider()
	require.NoError(t, p.Register(t.Context(), c))

	err := p.Boot(t.Context(), c)
	assert.ErrorIs(t, err, microsandbox.ErrNotBuilt)
	assert.ErrorContains(t, err, "the engine cannot be started")

	require.NoError(t, p.Terminate(t.Context()), "there is no engine to stop")

	t.Run("settings it cannot make an engine from are refused first", func(t *testing.T) {
		t.Parallel()

		c := danceable.New(container.New())

		wrong := settings()
		wrong.AdvertiseHost = "workload-vmhost-01"

		require.NoError(t, c.Bind(func() *configs.WorkloadVMHost { return wrong }, provider.Singleton()))
		require.NoError(t, c.Bind(func(name string) *slog.Logger { return slog.New(slog.DiscardHandler) }, provider.Lazy()))

		assert.ErrorContains(t, NewProvider().Boot(t.Context(), c), "WORKLOAD_VMHOST_ADVERTISE_HOST")
	})
}
