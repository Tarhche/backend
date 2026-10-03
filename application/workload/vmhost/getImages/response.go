package getImages

import "github.com/khanzadimahdi/testproject/domain/workload/vm"

type Response struct {
	// Images are never nil: none is an empty list.
	Images []vm.Image
}
