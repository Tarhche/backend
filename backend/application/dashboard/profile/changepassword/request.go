package changepassword

type validationErrors map[string]string

type Request struct {
	UserUUID        string `json:"-"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
