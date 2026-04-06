package providers

import (
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

func (p *jwtProvider) Register(app *ioc.Application) error {
	privateKeyData := []byte(os.Getenv("PRIVATE_KEY"))
	privateKey, err := ecdsa.ParsePrivateKey(privateKeyData)
	if err != nil {
		return err
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())

	return app.Container.Singleton(func() *jwt.JWT { return j })
}

func (p *jwtProvider) Boot(app *ioc.Application) error {
	return nil
}

func (p *jwtProvider) Terminate() error {
	return nil
}
