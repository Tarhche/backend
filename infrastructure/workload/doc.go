// Package workload reads what the dashboard shows of the workload out of what
// the blog's client of the control plane answers: what the workload refused
// (RefusalOf), and which stack made a container (StackIndexOf).
//
// RefusalOf reads what the workload refused out of the error it answered
// with, so a use case can hand it back the way it hands back its own
// validation: field by field, in the reader's language.
//
// The workload says no in two ways. The control plane refuses a request it
// would not take with codes by field — a quota, a bound, a Docker VM to choose
// — which it checks itself, since it is the one holding what they are checked
// against. A node refuses what it was asked about a VM it holds: a VM that is
// not running, one that is not a Docker VM, a dockerd that did not come up, a
// request dockerd itself turned down. Both are answers to the request rather
// than failures to answer it, so neither is an error by the time it leaves
// here.
//
// StackIndexOf says which stack made a container. A container knows the uuid
// of the stack whose graph made it, which its labels carry
// (docker.Container.Stack); the dashboard shows a container with its stack's
// slug and its uuid, and only when whoever asked can see that stack. It looks
// the stacks up once per request, and its StackIndex matches the two up, so a
// listing of containers costs one more question, not one per row.
package workload
