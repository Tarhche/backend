package providers

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/oauth"
)

// UseCase lists the accounts elsewhere that may be signed in with here, so that
// nothing has to guess which buttons to draw: a provider that was never
// configured is not offered.
type UseCase struct {
	providers oauth.Providers
}

func NewUseCase(providers oauth.Providers) *UseCase {
	return &UseCase{
		providers: providers,
	}
}

func (uc *UseCase) Execute(_ context.Context) (*Response, error) {
	return NewResponse(uc.providers.Names()), nil
}
