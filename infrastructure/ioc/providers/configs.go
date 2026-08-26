package providers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// configsProvider binds the application configuration into the container.
//
// The console has already filled the configuration structs from the flags and
// the environment by the time a provider runs, so this provider only has to
// hand them out. Every command registers it first, which is what makes the
// global configuration resolvable as *configs.Global; a command with settings
// of its own passes them here and they become resolvable the same way.
type configsProvider struct {
	commandConfigs []any
}

var _ provider.Provider = &configsProvider{}

// NewConfigsProvider creates the configuration provider. Each commandConfigs
// entry must be a pointer to the command's configuration struct, which is then
// resolvable by that pointer type.
func NewConfigsProvider(commandConfigs ...any) *configsProvider {
	return &configsProvider{commandConfigs: commandConfigs}
}

// Register binds the global configuration, then whatever the command added.
func (p *configsProvider) Register(ctx context.Context, c provider.Container) error {
	if err := bindConfigs(c, &configs.GlobalConfigs); err != nil {
		return err
	}

	for _, commandConfigs := range p.commandConfigs {
		if err := bindConfigs(c, commandConfigs); err != nil {
			return err
		}
	}

	return nil
}

func (p *configsProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *configsProvider) Terminate(ctx context.Context) error {
	return nil
}

// bindConfigs binds one configuration struct as a singleton under its own type.
//
// The container only accepts function resolvers, so the pointer is wrapped in a
// generated func() T returning it. That keeps the binding typed: a consumer
// resolves *configs.Blog and gets exactly the struct the flags were parsed
// into, pointer and all, so a value read after startup is the configured one.
func bindConfigs(c provider.Container, value any) error {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Pointer || reflected.IsNil() {
		return fmt.Errorf("providers: configuration must be a non-nil pointer to a struct, got %T", value)
	}

	resolver := reflect.MakeFunc(
		reflect.FuncOf(nil, []reflect.Type{reflected.Type()}, false),
		func([]reflect.Value) []reflect.Value { return []reflect.Value{reflected} },
	)

	return c.Bind(resolver.Interface(), provider.Singleton())
}
