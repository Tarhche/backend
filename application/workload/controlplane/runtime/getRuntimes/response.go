package getRuntimes

import "github.com/khanzadimahdi/testproject/domain/workload/runtime"

// Response is every class a task may be run with, in the order the platform
// names them.
type Response struct {
	Items []runtime.Availability `json:"items"`
}
