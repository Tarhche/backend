package files

type FileBson struct {
	UUID      string `bson:"_id,omitempty"`
	Name      string `bson:"name,omitempty"`
	Size      int64  `bson:"size,omitempty"`
	OwnerUUID string `bson:"owner_uuid,omitempty"`
	CreatedAt string `bson:"created_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
