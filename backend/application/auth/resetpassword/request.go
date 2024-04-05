package resetpassword

type validationErrors map[string]string

type Request struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
