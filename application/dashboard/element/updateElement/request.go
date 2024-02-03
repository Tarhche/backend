package updateelement

type validationErrors map[string]string

type Request struct {
	UUID   string   `json:"uuid"`
	Type   string   `json:"type"`
	Body   any      `json:"body"`
	Venues []string `json:"venues"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
