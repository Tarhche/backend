package logs

import (
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

type LogBson struct {
	ID          string    `bson:"_id"`
	TaskUUID    string    `bson:"task_uuid"`
	ContainerID string    `bson:"container_id,omitempty"`
	Stream      uint8     `bson:"stream"`
	Content     string    `bson:"content"`
	At          time.Time `bson:"at"`
}

// toLog reads a stored line back.
func toLog(l *LogBson) container.Log {
	return container.Log{
		TaskUUID:    l.TaskUUID,
		ContainerID: l.ContainerID,
		LogLine: container.LogLine{
			Stream:  container.Stream(l.Stream),
			Content: l.Content,
			At:      l.At,
		},
	}
}

// toBson prepares a line to be stored, deriving its id from the line itself.
//
// A worker that reconnects to a container's log stream resumes from a
// timestamp it has already shipped, so the lines around that point arrive
// twice. Identifying a line by its own content is what makes storing it twice
// a no-op rather than a duplicate.
func toBson(l *container.Log) LogBson {
	return LogBson{
		ID:          identify(l),
		TaskUUID:    l.TaskUUID,
		ContainerID: l.ContainerID,
		Stream:      uint8(l.Stream),
		Content:     l.Content,
		At:          l.At,
	}
}

func identify(l *container.Log) string {
	digest := sha256.New()

	digest.Write([]byte(l.TaskUUID))
	digest.Write([]byte{0})
	digest.Write([]byte(strconv.FormatInt(l.At.UTC().UnixNano(), 10)))
	digest.Write([]byte{0})
	digest.Write([]byte{uint8(l.Stream)})
	digest.Write([]byte{0})
	digest.Write([]byte(l.Content))

	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}
