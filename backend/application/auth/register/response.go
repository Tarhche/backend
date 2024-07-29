package register

type Response struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
}
