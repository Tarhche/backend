package changepassword

type ChangePasswordResponse struct {
	ValidationErrors validationErrors `json:"errors,omitempty"`
}
