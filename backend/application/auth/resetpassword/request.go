package resetpassword

type validationErrors map[string]string

type Request struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Token) == 0 {
		errors["token"] = "token is required"
	}

	if len(r.Password) == 0 {
		errors["password"] = "password is required"
	}

	return len(errors) == 0, errors
}
