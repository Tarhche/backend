package gettasks

import (
	gettask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type Response struct {
	Items      []gettask.Response `json:"items"`
	Pagination Pagination         `json:"pagination"`
}

type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(tasks []task.Task, totalPages, currentPage uint) *Response {
	items := make([]gettask.Response, len(tasks))

	for i, t := range tasks {
		items[i] = *gettask.NewResponse(t)
	}

	return &Response{
		Items: items,
		Pagination: Pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
