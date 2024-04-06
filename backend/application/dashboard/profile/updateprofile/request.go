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
	return true, nil
}
