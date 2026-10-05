package renameSnapshot

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		state   snapshot.State
		request Request
		want    domain.ValidationErrors
		name    string
		wantErr error
	}{
		"a stored snapshot is renamed": {
			state:   snapshot.Ready,
			request: Request{OwnerUUID: "owner", UUID: "s1", Name: " renamed "},
			name:    "renamed",
		},
		"one still being taken is not": {
			state:   snapshot.Creating,
			request: Request{UUID: "s1", Name: "renamed"},
			want:    domain.ValidationErrors{"snapshot": "invalid_state_transition"},
			name:    "taken",
		},
		"nor is it given no name": {
			state:   snapshot.Ready,
			request: Request{UUID: "s1", Name: "  "},
			want:    domain.ValidationErrors{"name": "required_field"},
			name:    "taken",
		},
		"somebody else's is not there": {
			state:   snapshot.Ready,
			request: Request{OwnerUUID: "other", UUID: "s1", Name: "mine"},
			wantErr: domain.ErrNotExists,
			name:    "taken",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repository := snapshotsMemory.NewRepository(snapshot.Snapshot{UUID: "s1", Name: "taken", OwnerUUID: "owner", State: tt.state})

			response, err := NewUseCase(repository, validator.New(translator.Codes{})).Execute(ctx, &tt.request)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, response.ValidationErrors)
			}

			stored, _ := repository.Stored("s1")
			assert.Equal(t, tt.name, stored.Name)
		})
	}
}
