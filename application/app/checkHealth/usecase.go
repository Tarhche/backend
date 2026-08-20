package checkhealth

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain"
)

// Dependency is an external dependency a service needs in order to serve
// traffic, together with the name it is reported under when it doesn't answer.
type Dependency struct {
	Name   string
	Pinger domain.Pinger
}

type UseCase struct {
	dependencies []Dependency
}

func NewUseCase(dependencies ...Dependency) *UseCase {
	return &UseCase{
		dependencies: dependencies,
	}
}

// Execute pings every dependency and returns nil when all of them answer. it
// stops at the first one that doesn't and returns its error wrapped in the
// dependency's name, so a failing probe says which dependency is at fault
// without waiting for the remaining ones to time out.
//
// each service passes the dependencies it actually has: the runner worker, for
// instance, talks to no database.
func (uc *UseCase) Execute(ctx context.Context) error {
	for _, dependency := range uc.dependencies {
		if err := dependency.Pinger.Ping(ctx); err != nil {
			return fmt.Errorf("%s: %w", dependency.Name, err)
		}
	}

	return nil
}
