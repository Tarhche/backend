// Package presenter is how the control plane's API pages what it lists. Every
// kind is shown as its manifest.
//
// The blog's control plane client reads exactly these shapes back, so a field
// renamed here is renamed there too.
package presenter

// Pagination says where a page is in its listing.
type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

// NewPagination is the page currentPage of a listing of total items, limit to
// a page.
func NewPagination(total uint, limit uint, currentPage uint) Pagination {
	totalPages := total / limit
	if totalPages*limit != total {
		totalPages++
	}

	return Pagination{TotalPages: totalPages, CurrentPage: currentPage}
}

// Offset is where page starts in a listing of limit to a page. Page zero is
// the first page, as page one is.
func Offset(page uint, limit uint) (uint, uint) {
	if page == 0 {
		page = 1
	}

	return (page - 1) * limit, page
}
