package getprofile

type Response struct {
	UUID         string `json:"uuid,omitempty"`
	Name         string `json:"name,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
	Email        string `json:"email,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`

	// ImpersonatedBy is who is seeing the dashboard as this user. It is absent
	// from an ordinary session, and is what lets the dashboard say whose eyes
	// these are without being told by whoever is holding the token.
	ImpersonatedBy *impersonatorResponse `json:"impersonated_by,omitempty"`
}

type impersonatorResponse struct {
	UUID     string `json:"uuid,omitempty"`
	Name     string `json:"name,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Username string `json:"username,omitempty"`
}
