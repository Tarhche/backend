package getusertasklogs

import (
	"time"
)

type Request struct {
	UUID      string    `json:"-"`
	OwnerUUID string    `json:"-"`
	After     time.Time `json:"after"`
	Limit     uint      `json:"limit"`
}
