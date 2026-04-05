package websocket

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
)

func TestInMemoryRequestRegistry(t *testing.T) {
	t.Parallel()

	t.Run("add and retrieve mappings", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(10)

		serverSideID, err := registry.Add("client-1")
		assert.NoError(t, err)
		assert.NotEmpty(t, serverSideID)

		clientSideID, err := registry.GetClientSideID(serverSideID)
		assert.NoError(t, err)
		assert.Equal(t, "client-1", clientSideID)

		gotServerSideID, err := registry.GetServerSideID("client-1")
		assert.NoError(t, err)
		assert.Equal(t, serverSideID, gotServerSideID)
	})

	t.Run("add duplicate client ID returns error", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(10)

		serverSideID1, err := registry.Add("client-1")
		assert.NoError(t, err)
		assert.NotEmpty(t, serverSideID1)

		serverSideID2, err := registry.Add("client-1")
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.Equal(t, serverSideID1, serverSideID2)
	})

	t.Run("get client side ID for non-existing server ID", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(10)

		clientSideID, err := registry.GetClientSideID("non-existing")
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, clientSideID)
	})

	t.Run("get server side ID for non-existing client ID", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(10)

		serverSideID, err := registry.GetServerSideID("non-existing")
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, serverSideID)
	})

	t.Run("delete by server side ID", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(10)

		serverSideID, err := registry.Add("client-1")
		assert.NoError(t, err)

		err = registry.DeleteByServerSideID(serverSideID)
		assert.NoError(t, err)

		// both lookups should now fail
		_, err = registry.GetClientSideID(serverSideID)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = registry.GetServerSideID("client-1")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("delete non-existing server side ID returns error", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(10)

		err := registry.DeleteByServerSideID("non-existing")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("add generates unique server IDs", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(10)

		id1, err := registry.Add("client-1")
		assert.NoError(t, err)

		id2, err := registry.Add("client-2")
		assert.NoError(t, err)

		assert.NotEqual(t, id1, id2)
	})
}
