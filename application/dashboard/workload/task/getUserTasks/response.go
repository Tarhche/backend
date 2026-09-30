package getusertasks

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
)

type Response struct {
	Items      []presenter.Task     `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
