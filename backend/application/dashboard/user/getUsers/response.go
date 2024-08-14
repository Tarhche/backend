package getusers

import "github.com/khanzadimahdi/testproject/domain/user"

type Response struct {
	Items      []userResponse `json:"items"`
	Pagination pagination     `json:"pagination"`
}

type userResponse struct {
	UUID     string `json:"uuid,omitempty"`
	Name     string `json:"name,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(u []user.User, totalPages, currentPage uint) *Response {
	items := make([]userResponse, len(u))

	for i := range u {
		items[i].UUID = u[i].UUID
		items[i].Name = u[i].Name
		items[i].Avatar = u[i].Avatar
		items[i].Email = u[i].Email
		items[i].Username = u[i].Username
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
