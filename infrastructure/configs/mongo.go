package configs

import "fmt"

const (
	defaultMongoScheme = "mongodb"
	defaultMongoHost   = "localhost"
	defaultMongoPort   = "27017"
)

// Mongo holds the settings the MongoDB connection is opened with.
type Mongo struct {
	Scheme       string `usage:"MongoDB connection scheme." env:"MONGO_SCHEME" long:"mongo-scheme"`
	Username     string `usage:"MongoDB user." env:"MONGO_USERNAME" long:"mongo-username"`
	Password     string `usage:"MongoDB password." env:"MONGO_PASSWORD" long:"mongo-password"`
	Host         string `usage:"MongoDB host." env:"MONGO_HOST" long:"mongo-host"`
	Port         string `usage:"MongoDB port." env:"MONGO_PORT" long:"mongo-port"`
	DatabaseName string `usage:"MongoDB database name." env:"MONGO_DATABASE_NAME" long:"mongo-database-name"`
}

// defaultMongo returns the settings a MongoDB connection is opened with when
// nothing overrides them.
func defaultMongo() Mongo {
	return Mongo{
		Scheme: defaultMongoScheme,
		Host:   defaultMongoHost,
		Port:   defaultMongoPort,
	}
}

// URI builds the connection string the database is opened with.
func (c *Mongo) URI() string {
	return fmt.Sprintf("%s://%s:%s@%s:%s", c.Scheme, c.Username, c.Password, c.Host, c.Port)
}
