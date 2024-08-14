package updateuser

type validationErrors map[string]string

type Request struct {
	UserUUID string `json:"uuid"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.UserUUID) == 0 {
		errors["uuid"] = "universal unique identifier (uuid) is required"
	}

	if len(r.Email) == 0 {
		errors["email"] = "email is required"
	}

	if len(r.Name) == 0 {
		errors["name"] = "name is required"
	}

	return len(errors) == 0, errors
}
