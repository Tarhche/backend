package refresh

type validationErrors map[string]string

type Request struct {
	Token string `json:"token"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
