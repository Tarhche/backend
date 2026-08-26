package providers

import (
	"context"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
)

const MailFromAddress = "mailFromAddress"

type emailProvider struct{}

var _ provider.Provider = &emailProvider{}

func NewEmailProvider() *emailProvider {
	return &emailProvider{}
}

func (p *emailProvider) Register(ctx context.Context, c provider.Container) error {
	var blogConfigs *configs.Blog
	if err := c.Resolve(&blogConfigs); err != nil {
		return err
	}

	mailFromAddress := blogConfigs.MailFrom
	mailer := email.NewSMTP(email.Config{
		Auth: email.Auth{
			Username: blogConfigs.MailUsername,
			Password: blogConfigs.MailPassword,
		},
		Host: blogConfigs.MailHost,
		Port: blogConfigs.MailPort,
	})

	if err := c.Bind(func() domain.Mailer { return mailer }, provider.Singleton()); err != nil {
		return err
	}

	return c.Bind(func() string { return mailFromAddress }, provider.Singleton(), provider.WithName(MailFromAddress))
}

func (p *emailProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *emailProvider) Terminate(ctx context.Context) error {
	return nil
}
