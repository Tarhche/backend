package getVMLogs

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Lines []vm.LogLine `json:"lines"`

	// Truncated says there were more lines than a reply carries, and only the
	// last of them are here.
	Truncated bool `json:"truncated,omitempty"`
}
