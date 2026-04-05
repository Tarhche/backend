package providers

import (
	"os"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
)

type emailProvider struct{}

var _ ioc.ServiceProvider = &emailProvider{}

func NewEmailProvider() *emailProvider {
	return &emailProvider{}
}

func (p *emailProvider) Register(app *ioc.Application) error {
	mailFromAddress := os.Getenv("MAIL_SMTP_FROM")
	mailer := email.NewSMTP(email.Config{
		Auth: email.Auth{
			Username: os.Getenv("MAIL_SMTP_USERNAME"),
			Password: os.Getenv("MAIL_SMTP_PASSWORD"),
		},
		Host: os.Getenv("MAIL_SMTP_HOST"),
		Port: os.Getenv("MAIL_SMTP_PORT"),
	})

	app.Container.Singleton(func() domain.Mailer { return mailer })
	app.Container.Singleton(func() string { return mailFromAddress }, ioc.WithNameBinding("mailFromAddress"))

	return nil
}

func (p *emailProvider) Boot(app *ioc.Application) error {
	return nil
}

func (p *emailProvider) Terminate() error {
	return nil
}
