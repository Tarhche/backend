package getuserfiles

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/files"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting files", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository

			r = Request{
				OwnerUUID: "user-uuid",
				Page:      0,
			}

			f = []file.File{
				{
					UUID:      "test-uuid-01",
					Name:      "role-name-01",
					Size:      1000,
					OwnerUUID: "user-uuid-01",
				},
				{
					UUID: "test-uuid-02",
					Name: "role-name-02",
				},
				{Name: "role-name-03"},
			}

			expectedResponse = Response{
				Items: []fileResponse{
					{
						UUID:      f[0].UUID,
						Name:      f[0].Name,
						Size:      f[0].Size,
						OwnerUUID: f[0].OwnerUUID,
					},
					{
						UUID:      f[1].UUID,
						Name:      f[1].Name,
						Size:      f[1].Size,
						OwnerUUID: f[1].OwnerUUID,
					},
					{
						UUID:      f[2].UUID,
						Name:      f[2].Name,
						Size:      f[2].Size,
						OwnerUUID: f[2].OwnerUUID,
					},
				},
				Pagination: pagination{
					TotalPages:  1,
					CurrentPage: 1,
				},
			}
		)

		filesRepository.On("CountByOwnerUUID", r.OwnerUUID).Once().Return(uint(len(f)), nil)
		filesRepository.On("GetAllByOwnerUUID", r.OwnerUUID, uint(0), uint(10)).Once().Return(f, nil)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("failure on counting files", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository

			r = Request{
				OwnerUUID: "user-uuid",
				Page:      0,
			}

			expectedError = errors.New("error")
		)

		filesRepository.On("CountByOwnerUUID", r.OwnerUUID).Once().Return(uint(0), expectedError)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository).Execute(&r)

		filesRepository.AssertNotCalled(t, "GetAll")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("failure on getting files", func(t *testing.T) {
		t.Parallel()

		var (
			filesRepository files.MockFilesRepository

			r = Request{
				OwnerUUID: "user-uuid",
				Page:      0,
			}

			expectedError = errors.New("error")
		)

		filesRepository.On("CountByOwnerUUID", r.OwnerUUID).Once().Return(uint(3), nil)
		filesRepository.On("GetAllByOwnerUUID", r.OwnerUUID, uint(0), uint(10)).Once().Return(nil, expectedError)
		defer filesRepository.AssertExpectations(t)

		response, err := NewUseCase(&filesRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
