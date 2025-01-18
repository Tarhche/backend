package getuserfiles

type Request struct {
	OwnerUUID string `json:"_"`
	Page      uint
}
