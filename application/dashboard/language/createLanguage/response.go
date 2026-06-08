package createlanguage

import "github.com/khanzadimahdi/testproject/domain"

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Code string `json:"code,omitempty"`
}
