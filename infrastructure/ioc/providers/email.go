package providers

import (
	"context"
	"os"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
)

const MailFromAddress = "mailFromAddress"

type emailProvider struct{}

var _ provider.Provider = &emailProvider{}

func NewEmailProvider() *emailProvider {
	return &emailProvider{}
}

func (p *emailProvider) Register(ctx context.Context, c provider.Container) error {
	mailFromAddress := os.Getenv("MAIL_SMTP_FROM")
	mailer := email.NewSMTP(email.Config{
		Auth: email.Auth{
			Username: os.Getenv("MAIL_SMTP_USERNAME"),
			Password: os.Getenv("MAIL_SMTP_PASSWORD"),
		},
		Host: os.Getenv("MAIL_SMTP_HOST"),
		Port: os.Getenv("MAIL_SMTP_PORT"),
	})

	if err := c.Bind(func() domain.Mailer { return mailer }, bind.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() string { return mailFromAddress }, bind.Singleton(), bind.WithName(MailFromAddress))
}

func (p *emailProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *emailProvider) Terminate(ctx context.Context) error {
	return nil
}
