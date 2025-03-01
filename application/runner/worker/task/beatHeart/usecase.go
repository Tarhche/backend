package beatHeart

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type UseCase struct {
	containerManager container.Manager
	messagePublisher domain.Publisher
	nodeName         string
}

func NewUseCase(
	containerManager container.Manager,
	messagePublisher domain.Publisher,
	nodeName string,
) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		messagePublisher: messagePublisher,
		nodeName:         nodeName,
	}
}

func (uc *UseCase) Execute(ctx context.Context) error {
	allContainers, err := uc.containerManager.GetByLabel(container.NodeNameLabelKey, uc.nodeName)
	if err != nil {
		return err
	}

	var eventBuffer bytes.Buffer
	var logsBuffer bytes.Buffer

	for _, c := range allContainers {
		if err := uc.containerManager.Logs(c.ID, &logsBuffer); err != nil {
			log.Println(err)
		}

		var logs []byte
		if logsBuffer.Len() > 0 {
			logs = make([]byte, logsBuffer.Len())
			if _, err := logsBuffer.Read(logs); err != nil {
				return err
			}
		}

		event := events.Heartbeat{
			UUID:          c.Labels[container.TaskUUIDLabelKey],
			Name:          c.Labels[container.TaskNameLabelKey],
			Image:         c.Image,
			ContainerUUID: c.ID,
			State:         int(uc.containerManager.EvaluateTaskState(c.Status)),
			Logs:          logs,
			At:            time.Now(),
		}

		if err := json.NewEncoder(&eventBuffer).Encode(event); err != nil {
			return err
		}

		payload := make([]byte, eventBuffer.Len())
		if _, err := eventBuffer.Read(payload); err != nil {
			return err
		}

		if err := uc.messagePublisher.Publish(context.Background(), events.HeartbeatName, payload); err != nil {
			return err
		}
	}

	return nil
}
