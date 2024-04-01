package getfiles

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/file"
)

type fileResponse struct {
	UUID      string    `json:"uuid"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	OwnerUUID string    `json:"owner_uuid"`
	CreatedAt time.Time `json:"created_at"`
}

type GetFilesReponse struct {
	Items      []fileResponse `json:"items"`
	Pagination pagination     `json:"pagination"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewGetFilesReponse(a []file.File, totalPages, currentPage uint) *GetFilesReponse {
	items := make([]fileResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Name = a[i].Name
		items[i].Size = a[i].Size
		items[i].OwnerUUID = a[i].OwnerUUID
	}

	return &GetFilesReponse{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
