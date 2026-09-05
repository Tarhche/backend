package watchTasks

import (
	gettask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
)

// Response is every container the watch covers, as it is now.
type Response struct {
	Items []gettask.Response `json:"items"`
}
