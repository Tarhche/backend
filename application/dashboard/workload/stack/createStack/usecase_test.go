package createStack

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

const compose = "services:\n  web:\n    image: nginx:1.27\n"

func useCase(workload *controlplane.MockClient) *UseCase {
	return NewUseCase(workload, workloadtest.Validator(), workloadtest.Translator(), workloadtest.Owners())
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deploys the compose file for whoever asks, and says where it went", func(t *testing.T) {
		t.Parallel()

		deploying := workloadtest.Stack()
		deploying.State = stack.Deploying
		deploying.Output = ""

		var workload controlplane.MockClient
		workload.On("CreateStack", mock.Anything, workloadtest.OwnerUUID, workloadControlPlane.StackRequest{
			Name:    "shop",
			Compose: compose,
		}).Once().Return(workloadControlPlane.CreatedStack{
			VM:    workloadControlPlane.ChosenVM{UUID: "vm-uuid", Name: "docker-1", Created: true},
			Stack: deploying,
		}, nil)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), &Request{
			Name:      "shop",
			Compose:   compose,
			OwnerUUID: workloadtest.OwnerUUID,
		})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"vm": {"uuid": "vm-uuid", "name": "docker-1", "created": true},
			"stack": {
				"uuid": "stack-uuid",
				"name": "shop",
				"slug": "shop-abcde",
				"owner_uuid": "owner-uuid",
				"owner": {"uuid": "owner-uuid", "name": "Mahdi", "username": "mahdi", "avatar": "avatar-uuid"},
				"vm_uuid": "vm-uuid",
				"compose": "services:\n  web:\n    image: nginx:1.27\n",
				"state": "deploying",
				"expected_state": "running",
				"created_at": "2026-10-04T12:00:00Z"
			}
		}`, string(presented))
	})

	t.Run("into the Docker VM it names, or one it describes", func(t *testing.T) {
		t.Parallel()

		var asked []workloadControlPlane.StackRequest

		var workload controlplane.MockClient
		workload.On("CreateStack", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Twice().
			Run(func(args mock.Arguments) { asked = append(asked, args.Get(2).(workloadControlPlane.StackRequest)) }).
			Return(workloadControlPlane.CreatedStack{Stack: workloadtest.Stack()}, nil)
		defer workload.AssertExpectations(t)

		_, err := useCase(&workload).Execute(context.Background(), &Request{Name: "shop", Compose: compose, VMUUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID})
		require.NoError(t, err)

		_, err = useCase(&workload).Execute(context.Background(), &Request{Name: "shop", Compose: compose, VM: &input.NewDockerVM{Name: "docker-2"}, OwnerUUID: workloadtest.OwnerUUID})
		require.NoError(t, err)

		require.Len(t, asked, 2)
		assert.Equal(t, workloadControlPlane.DockerVMChoice{UUID: "vm-uuid"}, asked[0].VM)
		assert.Equal(t, workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{Name: "docker-2"}}, asked[1].VM)
	})

	t.Run("a request the rules refuse never reaches the workload", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), &Request{
			Compose: strings.Repeat("#", MaxCompose+1),
			VMUUID:  "vm-uuid",
			VM:      &input.NewDockerVM{},
		})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{
			"name":    "this field is required",
			"compose": "this is larger than allowed",
			"vm":      "name a Docker VM or describe a new one, not both",
		}, response.ValidationErrors)

		workload.AssertNotCalled(t, "CreateStack", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a compose file of nothing but space is none", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, domain.ValidationErrors{"compose": "required_field"}, (&Request{Name: "shop", Compose: " \n "}).Validate())
		assert.Empty(t, (&Request{Name: "shop", Compose: strings.Repeat("#", MaxCompose)}).Validate(), "256 KiB is not too large")
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("CreateStack", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(workloadControlPlane.CreatedStack{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				Name:      "shop",
				Compose:   compose,
				OwnerUUID: workloadtest.OwnerUUID,
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Nil(t, response.VM)
			assert.Nil(t, response.Stack)
		})
	}
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Equal(t, domain.ValidationErrors{"name": "required_field", "compose": "required_field"}, (&Request{}).Validate())
}
