package stopusertask

type Request struct {
	UUID      string `json:"-"`
	OwnerUUID string `json:"-"`
}
