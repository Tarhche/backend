package providers

import (
	"context"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/khanzadimahdi/testproject/domain"
	roleContract "github.com/khanzadimahdi/testproject/domain/role"
	rolesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/roles"
)

type authProvider struct{}

var _ provider.Provider = &authProvider{}

func NewAuthProvider() *authProvider {
	return &authProvider{}
}

func (p *authProvider) Register(ctx context.Context, c provider.Container) error {
	if err := c.Bind(func(database *mongo.Database) roleContract.Repository {
		return rolesrepository.NewRepository(database)
	}, bind.Singleton()); err != nil {
		return err
	}

	return c.Bind(func(roleRepository roleContract.Repository) domain.Authorizer {
		return domain.NewRoleBasedAccessControl(roleRepository)
	}, bind.Singleton())
}

func (p *authProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *authProvider) Terminate(ctx context.Context) error {
	return nil
}
