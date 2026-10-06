package workload

import "github.com/khanzadimahdi/testproject/domain/workload/kind"

// nodeKinds are the kinds an orchestrator runs, each registered by its node
// strategy, which is all it takes for the node to carry out the kind's
// commands, answer its queries, report what it holds in every heartbeat, and
// route its streams and its ports.
//
// None is registered yet: the VMs, snapshots, stacks and tasks a node holds
// are carried out by their own use cases until their kinds take them over,
// each with one line here.
func nodeKinds(bindings ...kind.NodeBinding) (*kind.Registry[kind.NodeBinding], error) {
	kinds := kind.NewRegistry[kind.NodeBinding]()

	for _, binding := range bindings {
		if err := kinds.Register(binding); err != nil {
			return nil, err
		}
	}

	return kinds, nil
}
