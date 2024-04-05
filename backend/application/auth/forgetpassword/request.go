package forgetpassword

type validationErrors map[string]string

type Request struct {
	Identity string `json:"identity"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
