package providers

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

type validationProvider struct{}

var _ ioc.ServiceProvider = &validationProvider{}

func NewValidationProvider() *validationProvider {
	return &validationProvider{}
}

func (p *validationProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return iocContainer.Singleton(func(translator translatorContract.Translator) domain.Validator {
		return validator.New(translator)
	})
}

func (p *validationProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *validationProvider) Terminate() error {
	return p.Terminate()
}
