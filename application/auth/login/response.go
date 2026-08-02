package login

import "github.com/khanzadimahdi/testproject/domain"

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Set when the account is banned: the handler answers 403 with this instead
	// of tokens. Shaped like auth.BannedResponse, which every other refusal of a
	// banned user returns.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}
