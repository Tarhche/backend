package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserstack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/deleteUserStack"
	killuserstack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/killUserStack"
	restartuserstack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/restartUserStack"
	stopuserstack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/stopUserStack"
)

// The same commands, over one's own stacks: the stack is read as theirs
// before anything is asked of the runner, so one that is somebody else's is
// not found rather than refused.

// @Summary		Stop own stack
// @Description	stop every service of one of your own stacks
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/stacks/{uuid}/stop [post]
func NewStopUserHandler(useCase *stopuserstack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &stopuserstack.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}

// @Summary		Kill own stack
// @Description	stop every service of one of your own stacks at once
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/stacks/{uuid}/kill [post]
func NewKillUserHandler(useCase *killuserstack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &killuserstack.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}

// @Summary		Restart own stack
// @Description	restart every service of one of your own stacks
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/stacks/{uuid}/restart [post]
func NewRestartUserHandler(useCase *restartuserstack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &restartuserstack.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}

// @Summary		Delete own stack
// @Description	stop and remove one of your own stacks
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/stacks/{uuid} [delete]
func NewDeleteUserHandler(useCase *deleteuserstack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusNoContent,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &deleteuserstack.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}
