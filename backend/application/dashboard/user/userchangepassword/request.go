package userchangepassword

type validationErrors map[string]string

type Request struct {
	UserUUID    string `json:"-"`
	NewPassword string `json:"new_password"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.UserUUID) == 0 {
		errors["uuid"] = "universal unique identifier (uuid) is required"
	}

	if len(r.NewPassword) == 0 {
		errors["new_password"] = "password is required"
	}

	return len(errors) > 0, errors
}
