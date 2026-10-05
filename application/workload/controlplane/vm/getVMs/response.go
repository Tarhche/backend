package getVMs

import "github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"

type Response struct {
	Items      []presenter.VM       `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
