package providers

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

type validationProvider struct{}

var _ ioc.ServiceProvider = &validationProvider{}

func NewValidationProvider() *validationProvider {
	return &validationProvider{}
}

func (p *validationProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return iocContainer.Singleton(func(t translator.Translator) domain.Validator {
		return validator.New(t)
	})
}

func (p *validationProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *validationProvider) Terminate() error {
	return nil
}
