package getContainers

import "github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"

// Response is every container found, each with the Docker VM it is in.
type Response struct {
	Items []presenter.VMContainer `json:"items"`
}
