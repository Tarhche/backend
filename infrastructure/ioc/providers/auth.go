package providers

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	roleContract "github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	rolesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/roles"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type authProvider struct{}

var _ ioc.ServiceProvider = &authProvider{}

func NewAuthProvider() *authProvider {
	return &authProvider{}
}

func (p *authProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	if err := iocContainer.Singleton(func(database *mongo.Database) roleContract.Repository {
		return rolesrepository.NewRepository(database)
	}); err != nil {
		return err
	}

	return iocContainer.Singleton(func(roleRepository roleContract.Repository) domain.Authorizer {
		return domain.NewRoleBasedAccessControl(roleRepository)
	})
}

func (p *authProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *authProvider) Terminate() error {
	return nil
}
