package getVMs

import "github.com/khanzadimahdi/testproject/domain/workload/vm"

// Request is a page of VMs: one person's own when OwnerUUID is set, and only
// those of one kind when Kind is.
type Request struct {
	OwnerUUID string
	Kind      vm.Kind
	Page      uint
}
