package providers

import (
	"context"

	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

type translationProvider struct{}

var _ ioc.ServiceProvider = &translationProvider{}

func NewTranslationProvider() *translationProvider {
	return &translationProvider{}
}

func (p *translationProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	translator := translator.New(translation.Translations, translation.FA)

	return iocContainer.Singleton(func() translatorContract.Translator { return translator })
}

func (p *translationProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *translationProvider) Terminate() error {
	return nil
}
