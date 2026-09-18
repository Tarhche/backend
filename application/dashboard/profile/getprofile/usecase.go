package getprofile

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository user.Repository
}

func NewUseCase(userRepository user.Repository) *UseCase {
	return &UseCase{
		userRepository: userRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	u, err := uc.userRepository.GetOne(ctx, request.UserUUID)
	if err != nil {
		return nil, err
	}

	response := &Response{
		UUID:         request.UserUUID,
		Name:         u.Name,
		Avatar:       u.Avatar,
		Email:        u.Email,
		Username:     u.Username,
		LanguageCode: u.LanguageCode,
	}

	if len(request.ImpersonatorUUID) > 0 {
		if err := uc.loadImpersonator(ctx, request.ImpersonatorUUID, response); err != nil {
			return nil, err
		}
	}

	return response, nil
}

// loadImpersonator says who is seeing the dashboard as this user. It is a
// second person to look up, so it is a second thing that can fail.
func (uc *UseCase) loadImpersonator(ctx context.Context, impersonatorUUID string, response *Response) error {
	impersonator, err := uc.userRepository.GetOne(ctx, impersonatorUUID)
	if err != nil {
		return err
	}

	response.ImpersonatedBy = &impersonatorResponse{
		UUID:     impersonator.UUID,
		Name:     impersonator.Name,
		Avatar:   impersonator.Avatar,
		Username: impersonator.Username,
	}

	return nil
}
