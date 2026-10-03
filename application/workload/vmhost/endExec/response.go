package endExec

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

type Response struct {
	ValidationErrors domain.ValidationErrors
	Ended            guest.Ended
}
