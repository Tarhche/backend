package getConfig

type Response struct {
	Revision         uint     `json:"revision"`
	UserDefaultRoles []string `json:"user_default_roles"`
}
