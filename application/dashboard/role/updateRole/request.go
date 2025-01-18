package updaterole

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	UserUUIDs   []string `json:"user_uuids"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	if len(r.Description) == 0 {
		validationErrors["description"] = "required_field"
	}

	return validationErrors
}
