package providers

import (
	"context"
	"os"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type jwtProvider struct{}

var _ provider.Provider = &jwtProvider{}

func NewJwtProvider() *jwtProvider {
	return &jwtProvider{}
}

func (p *jwtProvider) Register(ctx context.Context, c provider.Container) error {
	privateKeyData := []byte(os.Getenv("PRIVATE_KEY"))
	privateKey, err := ecdsa.ParsePrivateKey(privateKeyData)
	if err != nil {
		return err
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())

	return c.Bind(func() *jwt.JWT { return j }, bind.Singleton())
}

func (p *jwtProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *jwtProvider) Terminate(ctx context.Context) error {
	return nil
}
