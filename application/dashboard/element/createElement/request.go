package createelement

type validationErrors map[string]string

type Request struct {
	Type   string
	Body   any
	Venues []string
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
