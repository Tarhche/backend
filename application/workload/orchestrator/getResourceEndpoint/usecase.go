// Package getResourceEndpoint says where a port of a resource this node
// holds is reached, whatever its kind.
package getResourceEndpoint

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// UseCase finds an endpoint by the slug a hostname carries, through the node
// strategy of the resource's kind: only the node holding a resource can see
// where its ports are, and its kind is what knows how to look.
//
// A slug this node holds nothing of the kind by, a port the resource does not
// expose, and a kind this node runs no such thing of, are domain.ErrNotExists;
// a resource that is here and cannot be reached now, one that is not
// running, is kind.ErrUnreachable, with why.
type UseCase struct {
	kinds *kind.Registry[kind.NodeBinding]
}

func NewUseCase(kinds *kind.Registry[kind.NodeBinding]) *UseCase {
	return &UseCase{kinds: kinds}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if len(request.Slug) == 0 {
		return nil, fmt.Errorf("%w: no slug names a %s", domain.ErrNotExists, request.Kind)
	}

	binding, runs := uc.kinds.Lookup(request.Kind)
	if !runs || !binding.Exposes() {
		return nil, fmt.Errorf("%w: this node serves no %s's ports", domain.ErrNotExists, request.Kind)
	}

	endpoint, err := binding.Endpoint(ctx, request.Slug, request.Port)
	if err != nil {
		return nil, err
	}

	// somewhere to send it is what an endpoint is.
	if len(endpoint.Address) == 0 {
		return nil, fmt.Errorf("%w: the %s %q does not expose that port", domain.ErrNotExists, request.Kind, request.Slug)
	}

	return &Response{Port: endpoint.Port, Address: endpoint.Address}, nil
}
