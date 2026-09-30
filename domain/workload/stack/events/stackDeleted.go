package events

import "time"

const StackDeletedName = "workloadStackDeleted"

type StackDeleted struct {
	UUID     string    `json:"uuid"`
	Slug     string    `json:"slug"`
	NodeName string    `json:"node_name"`
	At       time.Time `json:"at"`
}
