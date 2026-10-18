package getSnapshots

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
)

type Response struct {
	Items      []presenter.Snapshot `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
