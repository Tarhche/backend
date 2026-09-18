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

	if len(request.ImpersonatorUUID) == 0 {
		return response, nil
	}

	// whoever is behind the session is a second person to look up, and the
	// profile is still this user's whether or not that lookup says anything.
	impersonator, err := uc.userRepository.GetOne(ctx, request.ImpersonatorUUID)
	if err != nil {
		return nil, err
	}

	response.ImpersonatedBy = &impersonatorResponse{
		UUID:     impersonator.UUID,
		Name:     impersonator.Name,
		Avatar:   impersonator.Avatar,
		Username: impersonator.Username,
	}

	return response, nil
}
