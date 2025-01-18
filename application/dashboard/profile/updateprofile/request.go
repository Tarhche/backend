package updateprofile

type validationErrors map[string]string

type Request struct {
	UserUUID string `json:"-"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.UserUUID) == 0 {
		errors["uuid"] = "required_field"
	}

	if len(r.Name) == 0 {
		errors["name"] = "required_field"
	}

	if len(r.Email) == 0 {
		errors["email"] = "required_field"
	}

	if len(r.Username) == 0 {
		errors["username"] = "required_field"
	}

	return len(errors) == 0, errors
}
