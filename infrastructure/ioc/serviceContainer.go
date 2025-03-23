package ioc

import (
	binder "github.com/golobby/container/v3"
)

type BindingConfig struct {
	Lazy bool
	Name string
}

type BindingOption func(config *BindingConfig)

// WithLazyBinding is a function that returns a BindingOption with the Lazy field set to true.
func WithLazyBinding() BindingOption {
	return func(config *BindingConfig) {
		config.Lazy = true
	}
}

// WithName is a function that returns a BindingOption with the Name field set to the given name.
func WithNameBinding(name string) BindingOption {
	return func(config *BindingConfig) {
		config.Name = name
	}
}

type ResolvingConfig struct {
	Name string
}

type ResolvingOption func(options *ResolvingConfig)

// WithNameResolving is a function that returns a ResolvingOption with the Name field set to the given name.
func WithNameResolving(name string) ResolvingOption {
	return func(config *ResolvingConfig) {
		config.Name = name
	}
}

// ServiceContainer is a service container that can be used to resolve dependencies.
type ServiceContainer interface {
	// Singleton binds a singleton service.
	Singleton(resolver any, options ...BindingOption) error

	// Transient binds a transient service.
	Transient(resolver any, options ...BindingOption) error

	// Resolve resolves a service.
	Resolve(abstraction any, options ...ResolvingOption) error

	// Call calls a service.
	Call(receiver any) error
}

type serviceContainer struct {
	bindings binder.Container
}

var _ ServiceContainer = &serviceContainer{}

func NewContainer() *serviceContainer {
	return &serviceContainer{
		bindings: binder.New(),
	}
}

func (c *serviceContainer) Singleton(resolver any, options ...BindingOption) error {
	config := new(BindingConfig)
	for _, option := range options {
		option(config)
	}

	if len(config.Name) > 0 && config.Lazy {
		return c.bindings.NamedSingletonLazy(config.Name, resolver)
	}

	if len(config.Name) > 0 && !config.Lazy {
		return c.bindings.NamedSingleton(config.Name, resolver)
	}

	if len(config.Name) == 0 && config.Lazy {
		return c.bindings.SingletonLazy(resolver)
	}

	return c.bindings.Singleton(resolver)
}

func (c *serviceContainer) Transient(resolver any, options ...BindingOption) error {
	config := new(BindingConfig)
	for _, option := range options {
		option(config)
	}

	if len(config.Name) > 0 && config.Lazy {
		return c.bindings.NamedTransientLazy(config.Name, resolver)
	}

	if len(config.Name) > 0 && !config.Lazy {
		return c.bindings.NamedTransient(config.Name, resolver)
	}

	if len(config.Name) == 0 && config.Lazy {
		return c.bindings.TransientLazy(resolver)
	}

	return c.bindings.Transient(resolver)
}

func (c *serviceContainer) Resolve(abstraction any, options ...ResolvingOption) error {
	config := new(ResolvingConfig)
	for _, option := range options {
		option(config)
	}

	if len(config.Name) > 0 {
		return c.bindings.NamedResolve(abstraction, config.Name)
	}

	return c.bindings.Resolve(abstraction)
}

func (c *serviceContainer) Call(receiver any) error {
	return c.bindings.Call(receiver)
}
