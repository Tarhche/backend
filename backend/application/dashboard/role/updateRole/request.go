package updaterole

type validationErrors map[string]string

type Request struct {
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	UserUUIDs   []string `json:"user_uuids"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.UUID) == 0 {
		errors["uuid"] = "uuid is required"
	}

	if len(r.Name) == 0 {
		errors["name"] = "name is required"
	}

	if len(r.Description) == 0 {
		errors["description"] = "description is required"
	}

	return len(errors) == 0, errors
}
