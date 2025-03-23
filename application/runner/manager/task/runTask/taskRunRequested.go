package runTask

import (
	"encoding/json"
	"log"
)

type TaskRunRequested struct {
	usecase *UseCase
}

func NewTaskRunRequested(
	usecase *UseCase,
) *TaskRunRequested {
	return &TaskRunRequested{
		usecase: usecase,
	}
}

func (uc *TaskRunRequested) Handle(data []byte) error {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		log.Println("error unmarshalling request", err)

		return nil
	}

	// TODO: using usecase in handler ? (is this a good idea?)
	response, err := uc.usecase.Execute(&request)
	if len(response.ValidationErrors) > 0 {
		log.Println("validation errors", response.ValidationErrors)
	}

	return err
}
