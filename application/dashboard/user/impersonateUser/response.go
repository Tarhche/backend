package impersonateuser

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	// User is who the tokens are for. The caller has just been handed a session
	// that is not theirs, so it is told whose it is, and does not have to ask.
	User *userResponse `json:"user,omitempty"`
}

type userResponse struct {
	UUID     string `json:"uuid,omitempty"`
	Name     string `json:"name,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

func NewResponse(u user.User, accessToken string, refreshToken string) *Response {
	return &Response{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: &userResponse{
			UUID:     u.UUID,
			Name:     u.Name,
			Avatar:   u.Avatar,
			Email:    u.Email,
			Username: u.Username,
		},
	}
}
