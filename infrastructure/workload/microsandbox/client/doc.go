// Package client runs an orchestrator's tasks as microVMs, by asking the
// workload-microsandbox service for them. It is the orchestrator's half of
// microsandbox: Runtime is the workload's task.Runtime, NetworkManager its
// network.Manager and NodeManager its node.Manager, so nothing that runs,
// watches or reaches a task has to say anything different from what it says to
// docker.
//
// It is pure Go and links nothing of microsandbox's. Microsandbox has no
// daemon: whatever calls its SDK forks the VMs itself and judges them alive by
// their PIDs, so it has to share their container, and the SDK is a cgo library
// besides. The service is that process, in that container, and this package
// speaks to it in the language of infrastructure/workload/microsandbox/api,
// which is all the two sides share. The orchestrator stays a static binary that
// is redeployed as often as it likes, and every redeploy leaves the VMs where
// they are.
//
// # Reaching the service
//
// JSON over HTTPS, with mutual TLS under the workload's own authority. An
// orchestrator already holds a client certificate for the tunnel, signed by
// that authority, and that is the one it presents: there is no new secret to
// hand out. The service's certificate has to answer for the host of the
// service's URL, as the ingress's has to answer for the tunnel's server name.
//
// The client asks the service which version of the API it speaks before it
// asks anything else, and refuses one of another major version rather than
// guess at it. The two are deployed apart, the service rarely because
// redeploying it restarts every microVM, so they are seldom built from the same
// commit; within a version both ignore what they do not know.
//
// # What microsandbox cannot do
//
// Some tasks cannot run in a microVM, and saying so is the client's job, before
// anything reaches the service. The error is the task's failure reason, word
// for word, so these are written for the person reading it:
//
//   - a stack, since microsandbox has no network between sandboxes;
//   - a read-only root;
//   - no network at all, since microsandbox cannot make a sandbox without a
//     network interface, only one whose traffic is denied.
//
// Everything the service refuses comes back with the service's own message,
// which reaches the task the same way, and a run the service does not have is
// domain.ErrNotExists, as a container docker does not have is.
//
// # Ports
//
// A task's ports are published by the service, on the service's address, at
// host ports it picks itself. While a run is running they are reported as
// docker reports a container's, bound on 0.0.0.0, so the heartbeat and the port
// proxy read them unchanged; the orchestrator reaches them at the host of the
// service's URL, which is what configs.WorkloadOrchestrator.PortsHost says.
package client
