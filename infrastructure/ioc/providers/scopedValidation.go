package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

// scopedValidationProvider binds a request-scoped validator backed by the
// request-scoped translator, so validation messages are produced in the
// request's language. It must be registered after scopedTranslationProvider.
type scopedValidationProvider struct{}

var (
	_ provider.Provider = &scopedValidationProvider{}
	_ provider.HasScope = &scopedValidationProvider{}
)

func NewScopedValidationProvider() *scopedValidationProvider {
	return &scopedValidationProvider{}
}

func (p *scopedValidationProvider) Scoped() bool {
	return true
}

func (p *scopedValidationProvider) Register(ctx context.Context, c provider.Container) error {
	var t translator.Translator
	if err := c.Resolve(&t); err != nil {
		return err
	}

	v := validator.New(t)

	return c.Bind(func() domain.Validator { return v }, bind.Singleton())
}

func (p *scopedValidationProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *scopedValidationProvider) Terminate(ctx context.Context) error {
	return nil
}
