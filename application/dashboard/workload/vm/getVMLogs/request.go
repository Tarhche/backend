package getVMLogs

import (
	"time"
)

type Request struct {
	UUID      string `json:"-"`
	OwnerUUID string `json:"-"`

	// Since leaves out what was written before it; zero is from the start.
	Since time.Time `json:"since"`

	// Tail keeps only the last lines; zero is as many as one answer carries.
	Tail uint `json:"tail"`
}
