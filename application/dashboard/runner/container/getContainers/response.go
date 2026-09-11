package getContainers

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
)

type Response struct {
	Items      []presenter.Container `json:"items"`
	Pagination presenter.Pagination  `json:"pagination"`
}
