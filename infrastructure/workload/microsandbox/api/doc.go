// Package api is the wire contract between the orchestrator's microsandbox
// client and the workload-microsandbox service. It is microsandbox's own, not a
// contract for virtual machines in general: another runtime that needs a
// process of its own outside the orchestrator brings its own contract.
//
// The service exists because microsandbox has no daemon. Whatever calls its
// SDK forks the VMs itself and judges them alive by their PIDs, so it has to
// live in their container, and the SDK is a cgo library besides. The
// orchestrator stays a static binary that never links it, and asks the service
// for everything through what is written here. Both sides import this package
// and nothing else of each other's, which is why it imports nothing but the
// standard library.
//
// # Transport
//
// JSON over HTTPS, on an internal network only orchestrators join, with mutual
// TLS 1.3 under the workload's certificate authority. The service checks the
// client's chain and its clientAuth usage, and the client presents the
// certificate an orchestrator already holds for the tunnel. Nothing else
// authenticates a request and nothing scopes one to its caller: a run belongs
// to the node named in it, as a container on the shared docker daemon belongs
// to the node named in its labels.
//
// # Versions
//
// Every path carries /v1, and Info says which major version the service
// speaks. Within a major version a change is additive only, and both sides
// ignore the fields they do not know. That is what lets the two be deployed
// apart: orchestrators are redeployed often and the service rarely, because
// redeploying it restarts every microVM, so the two are seldom built from the
// same commit. A client refuses a service of another major version rather than
// guess at it.
//
// # Units
//
// Sizes are bytes and CPUs are cores, as everywhere else in the workload.
// Microsandbox counts memory in MiB and CPUs in whole vCPUs, and the service's
// SDK adapter converts at that edge and nowhere else, as
// infrastructure/workload/container does for docker.
//
// # Errors
//
// A request that fails is answered with an ErrorResponse: a code to act on and
// a message for people. A client takes not_found as the workload's own "does
// not exist" and passes anything else on with its message, which is how a
// task that could not be run learns why.
//
// # Repeating a call
//
// Calls repeat the way docker's do, so a client that lost an answer and asks
// again ends up where it meant to:
//
//   - creating a run under a name its node already uses is 409 name_in_use,
//     after which the orchestrator takes the run that is there, found by its
//     task;
//   - starting a running run, and stopping or killing an exited one, change
//     nothing and succeed;
//   - deleting a run, or ending an exec, that is not there is 404, which the
//     caller takes as done;
//   - pulling an image that is already cached changes nothing.
//
// The service never cancels creating or stopping a sandbox because the client
// went away. It works to deadlines of its own, because cancelling
// microsandbox's create leaves a stopped sandbox behind.
//
// # Streams
//
// Two routes answer with more than one value. RouteRunLogs streams
// newline-delimited JSON, one LogLine per line; see LogLine. RouteExec is a
// websocket; see ExecRequest for its frames.
package api
