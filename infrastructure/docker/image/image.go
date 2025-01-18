package image

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/khanzadimahdi/testproject/domain/runner"
	"github.com/pkg/errors"
)

type ImagePullStatus struct {
	Status         string `json:"status"`
	Error          string `json:"error"`
	Progress       string `json:"progress"`
	ProgressDetail struct {
		Current int `json:"current"`
		Total   int `json:"total"`
	} `json:"progressDetail"`
}

type manager struct {
	cli *client.Client
}

var _ runner.ImageManager = &manager{}

func NewManager(cli *client.Client) *manager {
	return &manager{
		cli: cli,
	}
}

// PullImage outputs to stdout the contents of the runner image.
func (m *manager) PullImage(ctx context.Context, imageName string) error {
	out, err := m.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return errors.New("DOCKER PULL")
	}

	defer func() {
		if err := out.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	fd := json.NewDecoder(out)
	var status *ImagePullStatus
	for {
		if err := fd.Decode(&status); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return errors.Wrap(err, "DOCKER PULL")
		}

		if status.Error != "" {
			return errors.Wrap(errors.New(status.Error), "DOCKER PULL")
		}

		// uncomment to log image pull status
		// fmt.Println(status)
	}

	return nil
}
