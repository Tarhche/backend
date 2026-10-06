// Package getEndpoint says where the port of a code-runner task this node
// holds can be reached. A VM's ports are the vm kind's (its Exposer), served
// under the kind's plural.
package getEndpoint

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase finds an endpoint by the slug a hostname carries.
//
// A task runs as an instance of this node's engine, labelled with its slug.
// Only the engine knows where a port is published, and it is asked every
// time. A port is reachable only when it is one of the instance's endpoints:
// the engine publishes the ports an instance was given and nothing else, and
// nothing at all of one whose ingress is denied, so a port that is not there
// is not exposed, whatever the request asks for.
type UseCase struct {
	engine vm.Engine
}

func NewUseCase(engine vm.Engine) *UseCase {
	return &UseCase{engine: engine}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if len(request.Slug) == 0 {
		return nil, ErrNotHeld
	}

	instances, err := uc.engine.List(ctx)
	if err != nil {
		return nil, err
	}

	held, found := bySlug(instances, request.Slug)
	if !found {
		return nil, ErrNotHeld
	}

	if held.State != vm.InstanceRunning {
		return nil, ErrNotRunning
	}

	endpoint, found := pick(held.Endpoints, request.Port)
	if !found {
		return nil, ErrNotExposed
	}

	return &Response{Port: endpoint.Port, Address: endpoint.Address}, nil
}

// bySlug is the task answering to slug. A task may have more than one
// instance — a retry is a new one beside what is left of the last — and the
// one running is the one to reach. A VM is not reached here, whatever its
// slug.
func bySlug(instances []vm.Instance, slug string) (vm.Instance, bool) {
	var (
		held  vm.Instance
		found bool
	)

	for _, instance := range instances {
		if instance.Labels[vm.LabelSlug] != slug {
			continue
		}

		if instance.Labels[vm.LabelPurpose] != vm.PurposeTask {
			continue
		}

		if !found || instance.State == vm.InstanceRunning {
			held, found = instance, true
		}
	}

	return held, found
}

// pick is the endpoint of the port a hostname asked for, or of the lowest one
// exposed when it named none, which is what a task with a single port needs
// no port in its name for.
func pick(endpoints []vm.Endpoint, requested port.Port) (vm.Endpoint, bool) {
	var (
		picked vm.Endpoint
		found  bool
	)

	for _, endpoint := range endpoints {
		if len(endpoint.Address) == 0 {
			continue
		}

		if requested > 0 {
			if endpoint.Port == requested {
				return endpoint, true
			}

			continue
		}

		if !found || endpoint.Port < picked.Port {
			picked, found = endpoint, true
		}
	}

	return picked, found
}
