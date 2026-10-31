package getResources

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Response is a page of manifests, newest first.
type Response struct {
	Items      []kind.Raw           `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
