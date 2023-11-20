package file

type File struct {
	UUID      string
	Name      string
	Size      int64
	OwnerUUID string
}

type Repository interface {
	GetOne(UUID string) (File, error)
	Save(*File) error
	Delete(UUID string) error
}
