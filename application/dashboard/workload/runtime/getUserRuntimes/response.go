package getUserRuntimes

import "github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"

// Response is every class one of the person's own tasks may be run with, in
// the order the workload names them.
type Response struct {
	Items []presenter.Runtime `json:"items"`
}
