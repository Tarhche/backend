package watchStacks

import (
	getstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStack"
)

// Response is every stack the watch covers, as it is now.
type Response struct {
	Items []getstack.Response `json:"items"`
}
