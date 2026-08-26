package providers

import (
	"context"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type jwtProvider struct{}

var _ provider.Provider = &jwtProvider{}

func NewJwtProvider() *jwtProvider {
	return &jwtProvider{}
}

func (p *jwtProvider) Register(ctx context.Context, c provider.Container) error {
	var blogConfigs *configs.Blog
	if err := c.Resolve(&blogConfigs); err != nil {
		return err
	}

	privateKey, err := ecdsa.ParsePrivateKey([]byte(blogConfigs.PrivateKey))
	if err != nil {
		return err
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())

	return c.Bind(func() *jwt.JWT { return j }, provider.Singleton())
}

func (p *jwtProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *jwtProvider) Terminate(ctx context.Context) error {
	return nil
}
