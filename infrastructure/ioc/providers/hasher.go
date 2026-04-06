package providers

import (
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
)

type hasherProvider struct{}

var _ ioc.ServiceProvider = &hasherProvider{}

func NewHasherProvider() *hasherProvider {
	return &hasherProvider{}
}

func (p *hasherProvider) Register(app *ioc.Application) error {
	hasher := argon2.NewArgon2id(3, 32*1024, 2, 64)

	return app.Container.Singleton(func() password.Hasher { return hasher })
}

func (p *hasherProvider) Boot(app *ioc.Application) error {
	return nil
}

func (p *hasherProvider) Terminate() error {
	return nil
}
