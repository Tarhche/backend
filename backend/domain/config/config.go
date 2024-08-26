package config

type Config struct {
	Revision             uint // to keep trace of config changes
	UserDefaultRoleUUIDs []string
}

type Repository interface {
	GetLatestRevision() (Config, error)
	Save(*Config) (string, error)
}
