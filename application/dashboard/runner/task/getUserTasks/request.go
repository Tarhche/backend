package getusertasks

type Request struct {
	Page      uint   `json:"page"`
	OwnerUUID string `json:"-"`
}
