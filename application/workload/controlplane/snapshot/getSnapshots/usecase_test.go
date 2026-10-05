package getSnapshots

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	useCase := NewUseCase(snapshotsMemory.NewRepository(
		snapshot.Snapshot{UUID: "01", OwnerUUID: "owner", VMUUID: "vm-a"},
		snapshot.Snapshot{UUID: "02", OwnerUUID: "owner", VMUUID: "vm-b"},
		snapshot.Snapshot{UUID: "03", OwnerUUID: "other", VMUUID: "vm-a"},
	))

	for name, tt := range map[string]struct {
		request Request
		want    []string
	}{
		"anybody's":                     {request: Request{}, want: []string{"03", "02", "01"}},
		"one person's":                  {request: Request{OwnerUUID: "owner"}, want: []string{"02", "01"}},
		"taken of one vm":               {request: Request{VMUUID: "vm-a"}, want: []string{"03", "01"}},
		"one person's, taken of one vm": {request: Request{OwnerUUID: "owner", VMUUID: "vm-a"}, want: []string{"01"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response, err := useCase.Execute(context.Background(), &tt.request)
			require.NoError(t, err)

			got := make([]string, len(response.Items))
			for i := range response.Items {
				got[i] = response.Items[i].UUID
			}

			assert.Equal(t, tt.want, got)
			assert.Equal(t, uint(1), response.Pagination.CurrentPage)
		})
	}
}
