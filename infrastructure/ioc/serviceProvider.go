package ioc

import (
	"context"
)

// ServiceProvider registers and boots services.
type ServiceProvider interface {
	// Register registers a service.
	Register(ctx context.Context, container ServiceContainer) error

	// Boot boots a service.
	Boot(ctx context.Context, container ServiceContainer) error

	// Terminate terminates corresponding services resources.
	Terminate() error
}
