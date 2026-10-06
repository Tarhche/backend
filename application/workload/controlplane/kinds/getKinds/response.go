package getKinds

import "github.com/khanzadimahdi/testproject/domain/workload/kind"

// Response is every kind the control plane runs, as each describes itself.
type Response struct {
	Items []kind.Descriptor `json:"items"`
}
