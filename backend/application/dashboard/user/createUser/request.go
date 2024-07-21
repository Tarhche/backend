package createuser

type validationErrors map[string]string

type Request struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Email) == 0 {
		errors["email"] = "email is required"
	}

	if len(r.Name) == 0 {
		errors["name"] = "name is required"
	}

	if len(r.Password) == 0 {
		errors["password"] = "password is required"
	}

	return len(errors) > 0, errors
}
