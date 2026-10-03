package getRuntimes

import "github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"

// Response is every class a task may be run with, in the order the workload
// names them.
type Response struct {
	Items []presenter.Runtime `json:"items"`
}
