package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
)

type hasherProvider struct{}

var _ provider.Provider = &hasherProvider{}

func NewHasherProvider() *hasherProvider {
	return &hasherProvider{}
}

func (p *hasherProvider) Register(ctx context.Context, c provider.Container) error {
	hasher := argon2.NewArgon2id(3, 32*1024, 2, 64)

	return c.Bind(func() password.Hasher { return hasher }, bind.Singleton())
}

func (p *hasherProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *hasherProvider) Terminate(ctx context.Context) error {
	return nil
}
