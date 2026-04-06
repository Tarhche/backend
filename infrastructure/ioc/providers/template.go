package providers

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
	"github.com/khanzadimahdi/testproject/resources/view"
)

type templateProvider struct{}

var _ ioc.ServiceProvider = &templateProvider{}

func NewTemplateProvider() *templateProvider {
	return &templateProvider{}
}

func (p *templateProvider) Register(app *ioc.Application) error {
	templateRenderer := template.NewRenderer(view.Files, "tmpl")

	return app.Container.Singleton(func() domain.Renderer { return templateRenderer })
}

func (p *templateProvider) Boot(app *ioc.Application) error {
	return nil
}

func (p *templateProvider) Terminate() error {
	return nil
}
