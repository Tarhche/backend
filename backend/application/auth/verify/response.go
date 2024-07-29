package verify

type Response struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
}
