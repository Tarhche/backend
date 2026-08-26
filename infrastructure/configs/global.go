// Package configs declares the application's configuration as plain structs.
//
// Every setting is a field carrying the flag and the environment variable it is
// read from, so the console populates them once at startup and nothing else in
// the application ever reads the environment for itself. The values reach their
// consumers through the dependency injection container, bound by the configs
// provider.
package configs

// Global holds the configuration every command reads, whichever one is invoked.
// Its nested structs are flattened into the same flag set, so each subsystem
// keeps its settings together without nesting the flags themselves.
type Global struct {
	Mongo     Mongo
	Nats      Nats
	Profiling Profiling
}

// GlobalConfigs holds the global configuration values for the application. The
// console fills it from the root flags and the environment before any command
// runs.
var GlobalConfigs Global = Global{
	Mongo:     defaultMongo(),
	Profiling: defaultProfiling(),
}
