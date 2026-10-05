package presenter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/user"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

func shop() stack.Stack {
	return stack.Stack{
		UUID:          "stack-uuid",
		Name:          "shop",
		OwnerUUID:     "owner-uuid",
		VMUUID:        "vm-uuid",
		Slug:          "shop-abcde",
		Compose:       "services:\n  web:\n    image: nginx\n",
		ExpectedState: stack.Running,
		State:         stack.Running,
		Output:        "Container shop-abcde-web-1  Started",
		CreatedAt:     at,
		UpdatedAt:     at.Add(30e9),
	}
}

func TestNewStack(t *testing.T) {
	t.Parallel()

	t.Run("one stack, with its compose file", func(t *testing.T) {
		t.Parallel()

		owners := NewOwners([]user.User{{UUID: "owner-uuid", Name: "Mahdi"}})

		presented, err := json.Marshal(NewStack(shop(), owners))
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"uuid": "stack-uuid",
			"name": "shop",
			"slug": "shop-abcde",
			"owner_uuid": "owner-uuid",
			"owner": {"uuid": "owner-uuid", "name": "Mahdi"},
			"vm_uuid": "vm-uuid",
			"compose": "services:\n  web:\n    image: nginx\n",
			"state": "running",
			"expected_state": "running",
			"output": "Container shop-abcde-web-1  Started",
			"created_at": "2026-10-04T12:00:00Z",
			"updated_at": "2026-10-04T12:00:30Z"
		}`, string(presented))
	})

	t.Run("a listing leaves the compose files out", func(t *testing.T) {
		t.Parallel()

		presented := NewStacks([]stack.Stack{shop()}, NewOwners(nil))

		require.Len(t, presented, 1)
		assert.Empty(t, presented[0].Compose)
		assert.Equal(t, "shop-abcde", presented[0].Slug)
	})
}

func TestNewStackDetail(t *testing.T) {
	t.Parallel()

	t.Run("a stack with the containers compose made for it", func(t *testing.T) {
		t.Parallel()

		presented := NewStackDetail(workloadControlPlane.StackDetail{
			Stack:      shop(),
			Containers: []docker.Container{{ID: "c0ffee", Stack: "shop-abcde", Service: "web"}},
		}, NewOwners(nil))

		require.Len(t, presented.Containers, 1)
		assert.Equal(t, "web", presented.Containers[0].Service)
		assert.Empty(t, presented.Note)
		assert.Equal(t, shop().Compose, presented.Compose)
	})

	t.Run("one whose VM is not running says so, rather than seeming to have none", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewStackDetail(workloadControlPlane.StackDetail{
			Stack:        shop(),
			VMNotRunning: true,
		}, NewOwners(nil)))
		require.NoError(t, err)

		var detail map[string]any
		require.NoError(t, json.Unmarshal(presented, &detail))

		assert.Equal(t, []any{}, detail["containers"])
		assert.Equal(t, "vm_not_running", detail["note"])
	})
}
