package getVM

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Response struct {
	ValidationErrors domain.ValidationErrors
	VM               vm.VM
}
