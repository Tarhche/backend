package providers

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	roleContract "github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	rolesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/roles"
	"go.mongodb.org/mongo-driver/mongo"
)

type authProvider struct{}

var _ ioc.ServiceProvider = &authProvider{}

func NewAuthProvider() *authProvider {
	return &authProvider{}
}

func (p *authProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return iocContainer.Singleton(func(database *mongo.Database) (roleContract.Repository, domain.Authorizer) {
		roleRepository := rolesrepository.NewRepository(database)
		authorizer := domain.NewRoleBasedAccessControl(roleRepository)

		return roleRepository, authorizer
	})
}

func (p *authProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *authProvider) Terminate() error {
	return p.Terminate()
}
