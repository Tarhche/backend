package domain

import "context"

// Pinger reports whether an external dependency (a database, a message broker, ...) is reachable.
// It returns nil when the dependency answers and the underlying error otherwise.
type Pinger interface {
	Ping(ctx context.Context) error
}
