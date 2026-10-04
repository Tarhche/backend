package getVMStats

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Response struct {
	ValidationErrors domain.ValidationErrors
	Stats            vm.Stats
}
