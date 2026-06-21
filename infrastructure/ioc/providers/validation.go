package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

// validationProvider binds the default (non request-scoped) validator, backed
// by the default translator. Request-scoped, language-aware validation is
// provided by scopedValidationProvider.
type validationProvider struct{}

var _ provider.Provider = &validationProvider{}

func NewValidationProvider() *validationProvider {
	return &validationProvider{}
}

func (p *validationProvider) Register(ctx context.Context, c provider.Container) error {
	return c.Bind(func(t translator.Translator) domain.Validator {
		return validator.New(t)
	}, bind.Singleton())
}

func (p *validationProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *validationProvider) Terminate(ctx context.Context) error {
	return nil
}
