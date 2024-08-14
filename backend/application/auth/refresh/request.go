package refresh

type validationErrors map[string]string

type Request struct {
	Token string `json:"token"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Token) == 0 {
		errors["token"] = "token is required"
	}

	return len(errors) == 0, errors
}
