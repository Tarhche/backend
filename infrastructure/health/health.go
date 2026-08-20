// Package health holds the pingers that report whether the external
// dependencies of a service are reachable.
package health

import "time"

// pingTimeout bounds how long a single ping may take. every pinger applies it,
// so a probe answers well within the container healthcheck's own timeout instead
// of waiting out a driver's much longer default. a context that already expires
// sooner keeps its own deadline.
//
// nats needs it for a second reason: FlushWithContext rejects a context that
// carries no deadline, and an http request's context doesn't have one.
const pingTimeout = 2 * time.Second
