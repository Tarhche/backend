package getResourceEndpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	holding := func() map[string]held {
		return map[string]held{
			"desk-abcde":  {lit: true, ports: map[port.Port]string{8080: "vmhost-01:20000", 80: "vmhost-01:20001"}},
			"porch-fghij": {lit: false, ports: map[port.Port]string{80: "vmhost-01:20002"}},
			"attic-klmno": {lit: true, ports: map[port.Port]string{80: ""}},
		}
	}

	for name, tt := range map[string]struct {
		strategy func() kind.Node[lampSpec, lampStatus]
		request  Request

		want *Response
		err  error
	}{
		"a bare request reaches a lamp's lowest port": {
			request: Request{Kind: "lamp", Slug: "desk-abcde"},
			want:    &Response{Port: 80, Address: "vmhost-01:20001"},
		},
		"a named port reaches that port": {
			request: Request{Kind: "lamp", Slug: "desk-abcde", Port: 8080},
			want:    &Response{Port: 8080, Address: "vmhost-01:20000"},
		},
		"a port the lamp does not expose is not there": {
			request: Request{Kind: "lamp", Slug: "desk-abcde", Port: 22},
			err:     domain.ErrNotExists,
		},
		"a lamp that is not lit cannot be reached now": {
			request: Request{Kind: "lamp", Slug: "porch-fghij"},
			err:     kind.ErrUnreachable,
		},
		"a slug this node holds nothing by is not there": {
			request: Request{Kind: "lamp", Slug: "other-pqrst"},
			err:     domain.ErrNotExists,
		},
		"nor is a port published nowhere": {
			request: Request{Kind: "lamp", Slug: "attic-klmno"},
			err:     domain.ErrNotExists,
		},
		"nor anything of a kind this node does not run": {
			request: Request{Kind: "kettle", Slug: "desk-abcde"},
			err:     domain.ErrNotExists,
		},
		"nor of a kind whose strategy serves no ports": {
			strategy: func() kind.Node[lampSpec, lampStatus] { return &lamps{held: holding()} },
			request:  Request{Kind: "lamp", Slug: "desk-abcde"},
			err:      domain.ErrNotExists,
		},
		"nor anything no slug names": {
			request: Request{Kind: "lamp"},
			err:     domain.ErrNotExists,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var strategy kind.Node[lampSpec, lampStatus] = &exposingLamps{lamps: lamps{held: holding()}}
			if tt.strategy != nil {
				strategy = tt.strategy()
			}

			response, err := NewUseCase(running(t, strategy)).Execute(t.Context(), &tt.request)

			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, response)
		})
	}
}
