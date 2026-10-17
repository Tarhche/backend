package attachResource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	holding := func() map[string]held {
		return map[string]held{
			"lamp-1": {owner: "owner-uuid", lit: true},
			"lamp-2": {owner: "owner-uuid", lit: false},
			"lamp-3": {owner: "", lit: true},
		}
	}

	for name, tt := range map[string]struct {
		strategy func() kind.Node[lampSpec, lampStatus]
		request  Request

		invalid domain.ValidationErrors
		err     error
		opened  []string
	}{
		"its owner's terminal is opened": {
			request: Request{Kind: "lamp", Action: "attach", UUID: "lamp-1", OwnerUUID: "owner-uuid"},
			opened:  []string{"attach lamp-1 owner-uuid"},
		},
		"somebody else's is not there for them": {
			request: Request{Kind: "lamp", Action: "attach", UUID: "lamp-1", OwnerUUID: "somebody-else"},
			err:     domain.ErrNotExists,
			opened:  []string{"attach lamp-1 somebody-else"},
		},
		"nobody is not its owner, and is not even looked for": {
			request: Request{Kind: "lamp", Action: "attach", UUID: "lamp-1"},
			err:     domain.ErrNotExists,
		},
		"a public stream is asked of its kind for nobody, which opens one of everybody's": {
			request: Request{Kind: "lamp", Action: "watch", UUID: "lamp-3"},
			opened:  []string{"watch lamp-3 "},
		},
		"and says one of somebody's is not there for nobody": {
			request: Request{Kind: "lamp", Action: "watch", UUID: "lamp-1"},
			err:     domain.ErrNotExists,
			opened:  []string{"watch lamp-1 "},
		},
		"a stream that is not public is not opened in one of everybody's for nobody": {
			request: Request{Kind: "lamp", Action: "attach", UUID: "lamp-3"},
			err:     domain.ErrNotExists,
		},
		"one that is there and not lit cannot be opened now": {
			request: Request{Kind: "lamp", Action: "attach", UUID: "lamp-2", OwnerUUID: "owner-uuid"},
			err:     kind.ErrUnreachable,
			opened:  []string{"attach lamp-2 owner-uuid"},
		},
		"one that is not held here is not there": {
			request: Request{Kind: "lamp", Action: "attach", UUID: "lamp-9", OwnerUUID: "owner-uuid"},
			err:     domain.ErrNotExists,
			opened:  []string{"attach lamp-9 owner-uuid"},
		},
		"an action that is not a stream is not there": {
			request: Request{Kind: "lamp", Action: "state", UUID: "lamp-1", OwnerUUID: "owner-uuid"},
			err:     domain.ErrNotExists,
		},
		"nor is anything of a kind this node does not run": {
			request: Request{Kind: "kettle", Action: "attach", UUID: "lamp-1", OwnerUUID: "owner-uuid"},
			err:     domain.ErrNotExists,
		},
		"nor of a kind whose strategy serves no streams": {
			strategy: func() kind.Node[lampSpec, lampStatus] { return &lamps{held: holding()} },
			request:  Request{Kind: "lamp", Action: "attach", UUID: "lamp-1", OwnerUUID: "owner-uuid"},
			err:      domain.ErrNotExists,
		},
		"a request that names nothing says so": {
			request: Request{Kind: "lamp", Action: "attach", OwnerUUID: "owner-uuid"},
			invalid: domain.ValidationErrors{"uuid": "required_field"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			attaching := &attachingLamps{lamps: lamps{held: holding()}}

			var strategy kind.Node[lampSpec, lampStatus] = attaching
			if tt.strategy != nil {
				strategy = tt.strategy()
			}

			session, invalid, err := NewUseCase(running(t, strategy), validates{}).Execute(t.Context(), &tt.request)

			assert.Equal(t, tt.opened, attaching.opened, "what the strategy was asked to open")

			switch {
			case tt.err != nil:
				assert.ErrorIs(t, err, tt.err)
				assert.Nil(t, session)
			case len(tt.invalid) > 0:
				require.NoError(t, err)
				assert.Equal(t, tt.invalid, invalid)
				assert.Nil(t, session)
			default:
				require.NoError(t, err)
				assert.Empty(t, invalid)
				assert.NotNil(t, session)
			}
		})
	}
}
