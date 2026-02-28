package permission

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getpermissions "github.com/khanzadimahdi/testproject/application/dashboard/permission/getPermissions"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/permissions"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("permissions", func(t *testing.T) {
		t.Parallel()

		var (
			permissionsRepository permissions.MockPermissionsRepository

			u = user.User{UUID: "auth-user-uuid"}

			p = []permission.Permission{
				{Name: "permission-name-01", Value: "permission-value-01"},
				{Name: "permission-name-02", Value: "permission-value-02"},
				{Name: "permission-name-03", Value: "permission-value-03"},
			}
		)

		permissionsRepository.On("GetAll").Once().Return(p)
		defer permissionsRepository.AssertExpectations(t)

		handler := NewIndexHandler(getpermissions.NewUseCase(&permissionsRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/index-permissions-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})
}
