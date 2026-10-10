package presenter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestTruncated(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name  string
		lines int
		tail  uint
		want  bool
	}{
		{name: "fewer lines than an answer carries is all of them", lines: 10, tail: 0, want: false},
		{name: "as many as an answer carries, asked for all, may have been cut", lines: noderequest.MaxLogLines, tail: 0, want: true},
		{name: "as many as an answer carries, asked for more, may have been cut", lines: noderequest.MaxLogLines, tail: 5000, want: true},
		{name: "as many as were asked for is what was asked for", lines: 500, tail: 500, want: false},
		{name: "asked for exactly as many as an answer carries, and given them", lines: noderequest.MaxLogLines, tail: noderequest.MaxLogLines, want: false},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Truncated(tt.lines, tt.tail))
		})
	}
}

func TestCommonShapes(t *testing.T) {
	t.Parallel()

	t.Run("where a listing sits in the whole", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewPagination(workloadControlPlane.Page[vm.VM]{TotalPages: 3, CurrentPage: 2}))
		require.NoError(t, err)

		assert.JSONEq(t, `{"total_pages": 3, "current_page": 2}`, string(presented))
	})

	t.Run("the VM something went into, and whether it was made for it", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewChosenVM(workloadControlPlane.ChosenVM{UUID: "vm-uuid", Name: "docker-1", Created: true}))
		require.NoError(t, err)

		assert.JSONEq(t, `{"uuid": "vm-uuid", "name": "docker-1", "created": true}`, string(presented))
	})

	t.Run("a VM's log, line by line, and where each came from", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewVMLogLines([]vm.LogLine{{At: at, Source: "kernel", Line: "booted"}}))
		require.NoError(t, err)

		assert.JSONEq(t, `[{"at": "2026-10-04T12:00:00Z", "source": "kernel", "line": "booted"}]`, string(presented))
	})
}
