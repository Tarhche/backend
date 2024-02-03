package createelement

type CreateElementResponse struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
	UUID             string           `json:"uuid,omitempty"`
}
