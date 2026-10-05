package getVMLogs

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	now := start
	e := memory.New(memory.WithClock(func() time.Time { return now }))

	_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindMachine, Image: "ubuntu:24.04"})
	require.NoError(t, err)

	lines := noderequest.MaxLogLines + 500
	for n := range lines {
		now = start.Add(time.Duration(n) * time.Second)
		require.NoError(t, e.Log("vm-1", vm.LogSourceKernel, "line "+strconv.Itoa(n)))
	}

	testcases := []struct {
		name    string
		request Request

		wantFirst     string
		wantCount     int
		wantTruncated bool
	}{
		{
			name:          "a log longer than a reply carries is its last lines, and says so",
			request:       Request{VMUUID: "vm-1"},
			wantFirst:     "line 500",
			wantCount:     noderequest.MaxLogLines,
			wantTruncated: true,
		},
		{
			name:      "the last lines asked for are what they are",
			request:   Request{VMUUID: "vm-1", Tail: 10},
			wantFirst: "line " + strconv.Itoa(lines-10),
			wantCount: 10,
		},
		{
			name:      "what was written since a moment fits",
			request:   Request{VMUUID: "vm-1", Since: start.Add(time.Duration(lines-3) * time.Second)},
			wantFirst: "line " + strconv.Itoa(lines-3),
			wantCount: 3,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			response, err := NewUseCase(e, validates{}).Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			require.Len(t, response.Lines, tt.wantCount)
			assert.Equal(t, tt.wantFirst, response.Lines[0].Line)
			assert.Equal(t, tt.wantTruncated, response.Truncated)
		})
	}

	t.Run("the log of a VM that is not here is not there", func(t *testing.T) {
		t.Parallel()

		_, err := NewUseCase(e, validates{}).Execute(t.Context(), &Request{VMUUID: "vm-2"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a request naming no VM is refused", func(t *testing.T) {
		t.Parallel()

		response, err := NewUseCase(e, validates{}).Execute(t.Context(), &Request{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm_uuid": "required_field"}, response.ValidationErrors)
	})
}
