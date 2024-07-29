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
	return true, nil
}
