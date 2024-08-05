package forgetpassword

type Response struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
}
