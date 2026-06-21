package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
	"github.com/khanzadimahdi/testproject/resources/view"
)

type templateProvider struct{}

var _ provider.Provider = &templateProvider{}

func NewTemplateProvider() *templateProvider {
	return &templateProvider{}
}

func (p *templateProvider) Register(ctx context.Context, c provider.Container) error {
	templateRenderer := template.NewRenderer(view.Files, "tmpl")

	return c.Bind(func() domain.Renderer { return templateRenderer }, bind.Singleton())
}

func (p *templateProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *templateProvider) Terminate(ctx context.Context) error {
	return nil
}
