package getusertasks

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
)

type Response struct {
	Items      []presenter.Task     `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
