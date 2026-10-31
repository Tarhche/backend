package getKinds_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("every kind run here, as it describes itself", func(t *testing.T) {
		t.Parallel()

		response, err := getKinds.NewUseCase(kindstest.Registry(&kindstest.Fans{})).Execute(context.Background())
		require.NoError(t, err)

		require.Len(t, response.Items, 1)
		assert.Equal(t, kindstest.Kind, response.Items[0].Name)

		described, err := json.Marshal(response)
		require.NoError(t, err)
		assert.Contains(t, string(described), `"allowed_in":["stopped","failed"]`, "the states each action is allowed in are said")
	})

	t.Run("and none, before any is", func(t *testing.T) {
		t.Parallel()

		response, err := getKinds.NewUseCase(kind.NewRegistry[kind.ControlPlaneBinding]()).Execute(context.Background())
		require.NoError(t, err)

		described, err := json.Marshal(response)
		require.NoError(t, err)
		assert.JSONEq(t, `{"items":[]}`, string(described))
	})
}
