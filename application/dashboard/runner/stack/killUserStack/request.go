package killuserstack

type Request struct {
	UUID      string `json:"-"`
	OwnerUUID string `json:"-"`
}
