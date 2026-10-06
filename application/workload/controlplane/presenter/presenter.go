// Package presenter is how the control plane's API shows a container:
// snake_case JSON, states as words and sizes as bytes. A kind on the
// framework, a VM, a snapshot or a stack, is shown as its manifest instead.
//
// The blog's control plane client reads exactly these shapes back, so a field
// renamed here is renamed there too.
package presenter

import (
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Pagination says where a page is in its listing.
type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

// NewPagination is the page currentPage of a listing of total items, limit to
// a page.
func NewPagination(total uint, limit uint, currentPage uint) Pagination {
	totalPages := total / limit
	if totalPages*limit != total {
		totalPages++
	}

	return Pagination{TotalPages: totalPages, CurrentPage: currentPage}
}

// Offset is where page starts in a listing of limit to a page. Page zero is
// the first page, as page one is.
func Offset(page uint, limit uint) (uint, uint) {
	if page == 0 {
		page = 1
	}

	return (page - 1) * limit, page
}

// Container is a container as a node reports it, in the shape node requests
// carry it.
type Container = noderequest.Container

func NewContainers(containers []docker.Container) []Container {
	items := make([]Container, len(containers))
	for i := range containers {
		items[i] = noderequest.NewContainer(containers[i])
	}

	return items
}

// ChosenVM is the Docker VM a container went into, and whether it was made
// for it.
type ChosenVM struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// VMContainer is a container and the Docker VM it is in.
type VMContainer struct {
	Container

	VMUUID string `json:"vm_uuid"`
	VMName string `json:"vm_name"`
}
