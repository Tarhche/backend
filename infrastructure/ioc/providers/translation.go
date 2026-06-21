package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

// translationProvider binds the default (non request-scoped) translator. It is
// used by paths that are not bound to a single HTTP request (the runner
// service, websocket replies, async defaults). Request-scoped, language-aware
// translation is provided by scopedTranslationProvider.
type translationProvider struct{}

var _ provider.Provider = &translationProvider{}

func NewTranslationProvider() *translationProvider {
	return &translationProvider{}
}

func (p *translationProvider) Register(ctx context.Context, c provider.Container) error {
	translator := translator.New(translation.Translations, translation.EN)

	return c.Bind(func() translatorContract.Translator { return translator }, bind.Singleton())
}

func (p *translationProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *translationProvider) Terminate(ctx context.Context) error {
	return nil
}
