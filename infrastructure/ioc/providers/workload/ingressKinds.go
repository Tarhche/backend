package workload

import "github.com/khanzadimahdi/testproject/domain/workload/kind"

// ingressKinds are the kinds the ingress finds the resources of, each
// registered by its ingress strategy: those with endpoints, whose ports are
// served under their slugs, and those with streams, such as a terminal. That
// is all it takes for the ingress to route them.
//
// None is registered yet: VMs and tasks are found by their own lookups until
// their kinds take them over, each with one line here.
func ingressKinds(bindings ...kind.IngressBinding) (*kind.Registry[kind.IngressBinding], error) {
	kinds := kind.NewRegistry[kind.IngressBinding]()

	for _, binding := range bindings {
		if err := kinds.Register(binding); err != nil {
			return nil, err
		}
	}

	return kinds, nil
}
