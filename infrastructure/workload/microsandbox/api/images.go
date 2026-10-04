package api

// PullRequest asks for an image to be in the service's cache, so that timing a
// task from its start does not time a pull. Asking for one that is already
// cached changes nothing.
type PullRequest struct {
	Reference string `json:"reference"`
}
