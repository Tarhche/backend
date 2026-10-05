package getSnapshots

import "github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"

type Response struct {
	Items      []presenter.Snapshot `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
