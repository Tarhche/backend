package getusers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting users succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			a = []user.User{
				{
					UUID:     "article-uuid-1",
					Name:     "John Doe",
					Email:    "johndoe@test.com",
					Username: "john.doe",
					PasswordHash: password.Hash{
						Value: make([]byte, 10),
						Salt:  make([]byte, 20),
					},
				},
				{
					UUID:     "article-uuid-2",
					Avatar:   "random-avatar",
					Username: "test-username",
				},
				{
					UUID: "article-uuid-3",
					Name: "test name",
				},
			}

			r = Request{
				Page: 0,
			}

			expectedResponse = Response{
				Items: []userResponse{
					{
						UUID:     a[0].UUID,
						Name:     a[0].Name,
						Avatar:   a[0].Avatar,
						Email:    a[0].Email,
						Username: a[0].Username,
					},
					{
						UUID:     a[1].UUID,
						Name:     a[1].Name,
						Avatar:   a[1].Avatar,
						Email:    a[1].Email,
						Username: a[1].Username,
					},
					{
						UUID:     a[2].UUID,
						Name:     a[2].Name,
						Avatar:   a[2].Avatar,
						Email:    a[2].Email,
						Username: a[2].Username,
					},
				},
				Pagination: pagination{
					CurrentPage: 1,
					TotalPages:  1,
				},
			}
		)

		userRepository.On("Count").Once().Return(uint(len(a)), nil)
		userRepository.On("GetAll", uint(0), uint(10)).Return(a, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting users fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("get articles failed")
		)

		userRepository.On("Count").Once().Return(uint(0), expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(&r)

		userRepository.AssertNotCalled(t, "GetAll")

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})

	t.Run("getting users fails", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository users.MockUsersRepository

			r = Request{
				Page: 0,
			}

			expectedErr = errors.New("get users failed")
		)

		userRepository.On("Count").Once().Return(uint(3), nil)
		userRepository.On("GetAll", uint(0), uint(10)).Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&userRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, response)
	})
}
