package client

// paginationPayload is where a page of a listing is in it, as the control
// plane's resource API says: every kind is a manifest, and every listing of
// one is paged so.
type paginationPayload struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}
