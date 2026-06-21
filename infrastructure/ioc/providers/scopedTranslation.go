package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/container/resolve"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/application/localize"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

// scopedTranslationProvider binds a request-scoped translator whose default
// locale is the language code seeded into the scope. It runs per-scope (not at
// global boot) so each request gets a translator for its own language.
type scopedTranslationProvider struct{}

var (
	_ provider.Provider = &scopedTranslationProvider{}
	_ provider.HasScope = &scopedTranslationProvider{}
)

func NewScopedTranslationProvider() *scopedTranslationProvider {
	return &scopedTranslationProvider{}
}

func (p *scopedTranslationProvider) Scoped() bool {
	return true
}

func (p *scopedTranslationProvider) Register(ctx context.Context, c provider.Container) error {
	var code string
	if err := c.Resolve(&code, resolve.WithName(localize.LanguageCode)); err != nil {
		return err
	}

	t := translator.New(translation.Translations, code)

	return c.Bind(func() translatorContract.Translator { return t }, bind.Singleton())
}

func (p *scopedTranslationProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *scopedTranslationProvider) Terminate(ctx context.Context) error {
	return nil
}
