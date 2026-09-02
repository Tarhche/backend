package events

import "time"

const StackDeletedName = "runnerStackDeleted"

// StackDeleted is what tells the node that ran a stack to drop the private
// network its services shared, once those services are gone.
type StackDeleted struct {
	UUID     string    `json:"uuid"`
	Slug     string    `json:"slug"`
	NodeName string    `json:"node_name"`
	At       time.Time `json:"at"`
}
