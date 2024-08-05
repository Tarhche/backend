package verify

type validationErrors map[string]string

type Request struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Repassword string `json:"repassword"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Token) == 0 {
		errors["token"] = "token is required"
	}

	if len(r.Name) == 0 {
		errors["name"] = "name is required"
	}

	if len(r.Username) == 0 {
		errors["username"] = "username is required"
	}

	if len(r.Password) == 0 {
		errors["password"] = "password is required"
	}

	if len(r.Repassword) == 0 || len(r.Password) != len(r.Repassword) || r.Password != r.Repassword {
		errors["repassword"] = "password and it's repeat should be same"
	}

	return len(errors) == 0, errors
}
