package getResources

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Response is a page of manifests, newest first, or why there is none: a
// word the kind does not say of its resources.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"-"`

	Items      []kind.Raw           `json:"items"`
	Pagination presenter.Pagination `json:"pagination"`
}
