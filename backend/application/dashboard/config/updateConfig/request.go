package updateConfig

type validationErrors map[string]string

type Request struct {
	UserDefaultRoles []string `json:"user_default_roles"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.UserDefaultRoles) == 0 {
		errors["user_default_roles"] = "user_default_roles is required"
	}

	return len(errors) == 0, errors
}
