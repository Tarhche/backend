package forgetpassword

type validationErrors map[string]string

type Request struct {
	Identity string `json:"identity"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Identity) == 0 {
		errors["identity"] = "identity is required"
	}

	return len(errors) == 0, errors
}
