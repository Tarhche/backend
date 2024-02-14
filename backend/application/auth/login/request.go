package login

type validationErrors map[string]string

type Request struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
