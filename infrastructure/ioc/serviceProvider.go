package ioc

import (
	"context"
)

type Application struct {
	Name        string
	Description string
	Arguments   []string
	Flags       map[string]string
	Container   ServiceContainer
	Ctx         context.Context
}

// ServiceProvider registers and boots services.
type ServiceProvider interface {
	// Register registers a service.
	Register(app *Application) error

	// Boot boots a service.
	Boot(app *Application) error

	// Terminate terminates corresponding services resources.
	Terminate() error
}
