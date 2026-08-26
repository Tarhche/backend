package configs

// Nats holds the settings the NATS connection is opened with.
type Nats struct {
	URL string `usage:"NATS server URL." env:"NATS_URL" long:"nats-url"`
}
