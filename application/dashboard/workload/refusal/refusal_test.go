package refusal

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

func TestOf(t *testing.T) {
	t.Parallel()

	unreachable := errors.New("the workload is unreachable")

	testcases := []struct {
		name    string
		locale  string
		err     error
		refused domain.ValidationErrors
		failure error
	}{
		{
			name: "no error is neither",
		},
		{
			name:    "what the control plane refuses is said field by field, in the reader's language",
			locale:  translation.EN,
			err:     &client.ValidationError{ValidationErrors: domain.ValidationErrors{"vm": "no_capacity", "compose": "too_large"}},
			refused: domain.ValidationErrors{"vm": "there is no room for it on any node right now, try again later", "compose": "this is larger than allowed"},
		},
		{
			name:    "and in Farsi to somebody reading Farsi",
			locale:  translation.FA,
			err:     &client.ValidationError{ValidationErrors: domain.ValidationErrors{"resources.memory": "quota_exceeded"}},
			refused: domain.ValidationErrors{"resources.memory": "این درخواست از سهمیهٔ شما بیشتر است"},
		},
		{
			name:    "a reason there are no words for is kept as it was given, rather than said as nothing",
			locale:  translation.EN,
			err:     &client.ValidationError{ValidationErrors: domain.ValidationErrors{"name": "must not end in a dash"}},
			refused: domain.ValidationErrors{"name": "must not end in a dash"},
		},
		{
			name:    "a refusal that names no field says nothing, so it is a failure",
			locale:  translation.EN,
			err:     &client.ValidationError{},
			failure: &client.ValidationError{},
		},
		{
			name:    "a VM that is not running is what was refused",
			locale:  translation.EN,
			err:     &noderequest.Error{Code: noderequest.CodeNotRunning},
			refused: domain.ValidationErrors{"vm": "the VM is not running"},
		},
		{
			name:    "a VM that is not a Docker VM",
			locale:  translation.EN,
			err:     &noderequest.Error{Code: noderequest.CodeNotDocker, Message: "kind machine"},
			refused: domain.ValidationErrors{"vm": "this VM is not a Docker VM"},
		},
		{
			name:    "a dockerd that did not come up in time",
			locale:  translation.EN,
			err:     &noderequest.Error{Code: noderequest.CodeDockerUnavailable},
			refused: domain.ValidationErrors{"vm": "Docker did not come up in the VM in time, try again shortly"},
		},
		{
			name:    "what dockerd turned down keeps docker's own words",
			locale:  translation.EN,
			err:     &noderequest.Error{Code: noderequest.CodeInvalid, Message: `Conflict. The container name "/web" is already in use`},
			refused: domain.ValidationErrors{"docker": `Docker refused the request: Conflict. The container name "/web" is already in use`},
		},
		{
			name:    "the domain's own refusals, however they are wrapped",
			locale:  translation.EN,
			err:     fmt.Errorf("creating: %w", vm.ErrNoCapacity),
			refused: domain.ValidationErrors{"vm": "there is no room for it on any node right now, try again later"},
		},
		{
			name:    "a snapshot from another engine is about the snapshot",
			locale:  translation.EN,
			err:     vm.ErrEngineMismatch,
			refused: domain.ValidationErrors{"snapshot_uuid": "the snapshot was taken by another engine and cannot be restored here"},
		},
		{
			name:    "a quota is about the VM",
			locale:  translation.EN,
			err:     vm.ErrQuotaExceeded,
			refused: domain.ValidationErrors{"vm": "this would take you past your quota"},
		},
		{
			name:    "dockerd refusing without words of its own is said all the same",
			locale:  translation.EN,
			err:     docker.ErrInvalid,
			refused: domain.ValidationErrors{"docker": "Docker refused the request"},
		},
		{
			name:    "something that is not there is not a refusal: it is not found",
			locale:  translation.EN,
			err:     &noderequest.Error{Code: noderequest.CodeNotFound},
			failure: domain.ErrNotExists,
		},
		{
			name:    "something that took too long is not a refusal either",
			locale:  translation.EN,
			err:     &noderequest.Error{Code: noderequest.CodeTimeout},
			failure: context.DeadlineExceeded,
		},
		{
			name:    "a node that failed is a failure",
			locale:  translation.EN,
			err:     &noderequest.Error{Code: noderequest.CodeInternal, Message: "boom"},
			failure: &noderequest.Error{Code: noderequest.CodeInternal, Message: "boom"},
		},
		{
			name:    "and so is a workload that could not be asked",
			locale:  translation.EN,
			err:     unreachable,
			failure: unreachable,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			refused, err := Of(tt.err, translator.New(translation.Translations, tt.locale))

			assert.Equal(t, tt.refused, refused)

			if tt.failure == nil {
				assert.NoError(t, err)

				return
			}

			assert.Error(t, err)
			if !errors.Is(err, tt.failure) {
				assert.Equal(t, tt.failure, err)
			}
		})
	}
}
