package config

import "context"

type Config struct {
	Revision             uint // to keep trace of config changes
	UserDefaultRoleUUIDs []string
	DefaultLanguageCode  string
}

type Repository interface {
	GetLatestRevision(ctx context.Context) (Config, error)
	Save(ctx context.Context, c *Config) (string, error)
}
