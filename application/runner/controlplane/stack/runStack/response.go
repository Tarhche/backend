package runStack

import "github.com/khanzadimahdi/testproject/domain"

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	UUID string `json:"uuid,omitempty"`
	Slug string `json:"slug,omitempty"`
}
