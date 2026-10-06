package getResource

import "github.com/khanzadimahdi/testproject/domain/workload/kind"

// Response is the resource's manifest.
type Response struct {
	Resource kind.Raw
}
