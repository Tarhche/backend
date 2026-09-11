package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteusercontainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/deleteUserContainer"
	killusercontainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/killUserContainer"
	restartusercontainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/restartUserContainer"
	stopusercontainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/stopUserContainer"
)

// The same commands, over one's own containers: the container is read as theirs
// before anything is asked of the runner, so one that is somebody else's is
// not found rather than refused.

// @Summary		Stop own container
// @Description	stop one of your own containers
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/containers/{uuid}/stop [post]
func NewStopUserHandler(useCase *stopusercontainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &stopusercontainer.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}

// @Summary		Kill own container
// @Description	stop one of your own containers at once
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/containers/{uuid}/kill [post]
func NewKillUserHandler(useCase *killusercontainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &killusercontainer.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}

// @Summary		Restart own container
// @Description	restart one of your own containers
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/containers/{uuid}/restart [post]
func NewRestartUserHandler(useCase *restartusercontainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &restartusercontainer.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}

// @Summary		Delete own container
// @Description	stop and remove one of your own containers
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/containers/{uuid} [delete]
func NewDeleteUserHandler(useCase *deleteusercontainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusNoContent,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &deleteusercontainer.Request{
				UUID:      r.PathValue("uuid"),
				OwnerUUID: auth.UUIDFromContext(r.Context()),
			})
		},
	}
}
