package providers

import (
	"context"
	"os"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type jwtProvider struct{}

var _ ioc.ServiceProvider = &jwtProvider{}

func NewJwtProvider() *jwtProvider {
	return &jwtProvider{}
}

func (p *jwtProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	privateKeyData := []byte(os.Getenv("PRIVATE_KEY"))
	privateKey, err := ecdsa.ParsePrivateKey(privateKeyData)
	if err != nil {
		return err
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())

	return iocContainer.Singleton(func() *jwt.JWT { return j })
}

func (p *jwtProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *jwtProvider) Terminate() error {
	return nil
}
