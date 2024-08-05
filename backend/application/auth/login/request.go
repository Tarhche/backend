package login

type validationErrors map[string]string

type Request struct {
	Identity string `json:"identity"`
	Password string `json:"password"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Identity) == 0 {
		errors["identity"] = "identity required"
	}

	if len(r.Password) == 0 {
		errors["password"] = "password required"
	}

	return len(errors) == 0, errors
}
