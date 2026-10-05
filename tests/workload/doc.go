// Package workload holds the workload's VMs to working end to end, in one
// process.
//
// Its tests run a control plane and a node the way their serve commands wire
// them, talking over a NATS server of the test's own, and drive them from the
// dashboard's use cases through the blog's client to the control plane's API,
// as a person using the dashboard would. What a test cannot run in a process
// is kept in memory instead: the control plane's records, the node's engine,
// the snapshots bucket and a Docker VM's dockerd. Everything between them —
// the messages and their subjects, the node's requests, the events and what
// becomes of them, and the vmhost the node reaches its engine through, on a
// unix socket — is the real thing, so a side that stops agreeing with
// another fails here, in plain go test, with no Docker and no KVM.
package workload
