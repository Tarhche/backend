package getContainerLogs

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Items []presenter.LogLine `json:"items"`

	// Truncated says there was more than one answer carries: what is here is
	// the last of it.
	Truncated bool `json:"truncated"`
}
