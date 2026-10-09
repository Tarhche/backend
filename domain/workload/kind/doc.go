// Package kind is what every workload is: a kind of resource, declared once,
// that every workload service runs with the same generic code. A VM, a
// snapshot, a stack, a code-runner task and the building blocks of a Docker
// VM are each a kind.
//
// A kind declares, in a Descriptor:
//
//   - its manifest. Every resource of every kind has one shape, Resource: its
//     Metadata, its kind's own spec, and its kind's own status, which embeds
//     Status, the part every kind shares: what it is doing, what it was asked
//     to be doing, and why it is failed or waiting.
//   - its Machine. Its states, which of them are in flight and which have
//     ended, and how a resource moves between them: on the actions it is
//     asked, and on what its node observes of it. A resource at rest is what
//     its node says it is; one in flight believes only the arrivals its
//     machine declares, so a stale report does not undo a command. Failed,
//     Deleted, Missing and Waiting are the states the framework itself moves
//     resources into.
//   - its Actions. What it can be asked; where each runs, on its node or in
//     the control plane; whether it is a command, answered later as a
//     ResourceActedOn, a query, answered at once, or a stream such as a
//     terminal; the states it is allowed in and the one it desires; the
//     permission it is asked under, workload.<plural>.<verb> and
//     its self. twin, or another kind's (PermissionsOf); how long its node
//     may take over a command, when that may be longer than the control
//     plane's patience (Timeout, a name each service sizes from its
//     settings); and the Codec its payload is read with, so a strategy is
//     handed the action's own struct.
//
// And it has a strategy in each service that runs it: ControlPlane, for
// admission, for deciding what to ask for when what a resource was asked to
// be and what it is differ, and for the actions run on its record; Node, for
// carrying out commands, answering queries, and its state, which is
// everything of the kind a node holds, and, as an Attacher and an Exposer,
// for its streams and its ports, and, as a Prompt, for being asked what it
// holds between beats too, when somebody waits on its changes as they
// happen; and Ingress, for where an instance is reached. The vmhost runs none: it stays the engine, and knows nothing of
// kinds.
//
// Every service runs one generic loop over a Registry of the kinds it runs,
// so a kind is added to a service by registering it there. A strategy is
// typed with its kind's own spec and status, and a registry holds kinds of
// every type, so a strategy is registered as a Binding, which speaks Raw: the
// Bind function of its service captures the types, and reads what arrives as
// JSON as the kind's own there, once.
//
// What the services tell each other is the same for every kind:
//
//   - an ActOnResource, on ActOnResourceName, from the control plane to the
//     node it names, carrying the resource as it was recorded so that a node
//     needs no database, and its ResourceActedOn, on ResourceActedOnName,
//     back;
//   - a Query, as a node request whose op is the kind and the action,
//     "stack.state", on the subject every node already answers on;
//   - and every kind's Report in the node's heartbeat: everything of the kind
//     the node holds, the parents it read and those it could not look inside,
//     so that what a report leaves out of a parent it read is known to be gone,
//     what lives in a parent it could not read is merely unseen, and what lives
//     in one it did not look inside at all, because it is not running, waits
//     on it.
//
// Registering a kind holds its descriptor to the rules every kind keeps
// (Check), so a kind that could not run is found out when its service is put
// together. kindtest.Conformance holds the kinds of every service together to
// the rest, in CI: a strategy wherever an action runs, and permissions that
// exist.
//
// Here are the framework's contracts and nothing that runs them. A kind
// declares itself in a package of its own, its strategies live with the
// services that run them, and the generic loops beside those.
package kind
