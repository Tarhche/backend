package getStacks

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
)

type Response struct {
	Items      []presenter.Stack    `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
