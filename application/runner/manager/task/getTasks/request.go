package gettasks

type Request struct {
	Page uint

	// OwnerUUID narrows the listing to one person's own containers. Empty is
	// everybody's, including the ones nobody owns.
	OwnerUUID string
}
