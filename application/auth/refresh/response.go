package refresh

import "github.com/khanzadimahdi/testproject/domain"

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Set when the account was banned after the refresh token was issued: the
	// handler answers 403 with this instead of renewing the session.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}
