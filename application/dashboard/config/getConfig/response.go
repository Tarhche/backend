package getConfig

type Response struct {
	Revision            uint     `json:"revision"`
	UserDefaultRoles    []string `json:"user_default_roles"`
	DefaultLanguageCode string   `json:"default_language_code"`
}
