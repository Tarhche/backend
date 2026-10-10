package stack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/netip"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// MaxFile is the most a stack's file may be, in bytes.
const MaxFile = 256 << 10

// FileField is the field of a stack's spec its file is asked for under, and
// what everything wrong with the file is said under: the file itself, or a
// path in it beneath it, compose.services.web.image.
const FileField = "compose"

// What a service of a stack's file may say.
const (
	// RestartNo is a service whose container is not started again once it
	// ends, the default, and RestartUnlessStopped one its VM's dockerd starts
	// again whenever it ends, but for being stopped.
	RestartNo            = "no"
	RestartUnlessStopped = "unless-stopped"

	// ConditionStarted is the one thing a service waits for of a service it
	// depends on: that it was started.
	ConditionStarted = "service_started"

	// DefaultNetwork is the network a service that names none joins, which a
	// file need not declare.
	DefaultNetwork = "default"

	// MountBind mounts a path of the VM into a service's container, and
	// MountVolume a volume the file declares.
	MountBind   = "bind"
	MountVolume = "volume"

	// ProtocolTCP and ProtocolUDP are what a port is published for, TCP when
	// it says nothing.
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
)

// The codes what is wrong with a stack's file is said in, beside those every
// request shares: required_field, invalid_value, invalid_name, invalid_image,
// invalid_ip, invalid_protocol, invalid_mount and too_large.
const (
	// CodeUnsupportedKey is a key the file does not take: one compose takes
	// that the platform does not carry out, such as build or healthcheck, and
	// any other.
	CodeUnsupportedKey = "unsupported_key"

	// CodeDuplicateKey is a key a mapping of the file says twice.
	CodeDuplicateKey = "duplicate_key"

	// CodeUndefinedService, CodeUndefinedVolume and CodeUndefinedNetwork are a
	// service, a volume and a network the file names and does not have.
	CodeUndefinedService = "undefined_service"
	CodeUndefinedVolume  = "undefined_volume"
	CodeUndefinedNetwork = "undefined_network"

	// CodeInvalidRestart is a restart that is neither no nor unless-stopped.
	CodeInvalidRestart = "invalid_restart"

	// CodeInvalidPortMapping is a port that cannot be read as one.
	CodeInvalidPortMapping = "invalid_port_mapping"

	// CodeInvalidCPUs and CodeInvalidMemory are limits that are not a number
	// above nothing, of cores and of bytes.
	CodeInvalidCPUs   = "invalid_cpus"
	CodeInvalidMemory = "invalid_memory"

	// CodeRelativeBind is a bind of a path that is not absolute,
	// CodeAnonymousVolume a volume mounted with no name, and
	// CodeInvalidVolumeType a mount that is neither a bind nor a volume.
	CodeRelativeBind      = "relative_bind"
	CodeAnonymousVolume   = "anonymous_volume"
	CodeInvalidVolumeType = "invalid_volume_type"

	// CodeInvalidCondition is a dependency waited on for anything but having
	// started.
	CodeInvalidCondition = "invalid_condition"

	// CodeDependsOnCycle is a service in a ring of services that depend on
	// one another, which no order starts.
	CodeDependsOnCycle = "depends_on_cycle"
)

// maxRange is the most ports one entry of a service's ports publishes.
const maxRange = 1000

// File is a stack's file as the platform reads it: what compose would read in
// it, of what the platform carries out itself, and nothing else. Its services
// are each a container, its volumes and networks each a volume and a network,
// made in the stack's Docker VM, and nothing is interpolated: a value is what
// it says, a dollar among it.
type File struct {
	// Services are what it runs, by their names.
	Services map[string]Service

	// Volumes and Networks are the volumes and networks it declares, in the
	// order of their names. A service that names no network joins the
	// default one, which needs no declaring.
	Volumes  []string
	Networks []string
}

// Service is one service of a stack's file: a container, as the file asks for
// it.
type Service struct {
	// Image is what it runs, pulled when its VM does not hold it: nothing is
	// built.
	Image string

	// Command and Entrypoint are each its words, as a shell would split a
	// line of them, none when it says nothing.
	Command    []string
	Entrypoint []string

	// Environment is each variable as KEY=value, in the order it says them.
	Environment []string

	Ports      []Port
	Privileged bool

	// Restart is no or unless-stopped.
	Restart string

	// DependsOn are the services it is started after, in the order of their
	// names: each once it has started.
	DependsOn []string

	Volumes []Mount

	// Networks are the networks it joins, in the order of their names: the
	// default one when it names none.
	Networks []string

	// PidsLimit is the most processes it runs at once, nothing leaving it to
	// its VM's dockerd; User whom it runs as, as written; and Runtime the
	// runtime its VM's dockerd runs it with.
	PidsLimit int64
	User      string
	Runtime   string

	// CPUs is in cores and Memory in bytes: nothing is no limit beyond its
	// VM's.
	CPUs   float64
	Memory uint64
}

// Port is a port of a service's container published on its VM: one port of a
// range is one of these. A host port of nothing is one docker picks.
type Port struct {
	Container port.Port
	Host      port.Port

	// HostIP is the VM's address it is published on; nothing is every one.
	HostIP string

	// Protocol is tcp or udp.
	Protocol string
}

// Mount is what is mounted into a service's container: a path of its VM, a
// bind, or a volume the file declares, by its name.
type Mount struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

// ServiceNames are the file's services, in the order of their names.
func (f File) ServiceNames() []string {
	return slices.Sorted(maps.Keys(f.Services))
}

// NetworksMade are the networks a stack of the file makes, in the order of
// their names: every one it declares, and the default one when a service
// joins it.
func (f File) NetworksMade() []string {
	made := slices.Clone(f.Networks)

	for _, service := range f.Services {
		for _, network := range service.Networks {
			if !slices.Contains(made, network) {
				made = append(made, network)
			}
		}
	}

	slices.Sort(made)

	return made
}

// Levels are the file's services in the order they are made and started in,
// a level at a time, as a stack's plan makes their containers: the first
// level what depends on nothing, and each after it what depends on nothing
// but the levels before it, each in the order of its names. A service in a
// ring of services that depend on one another is in none, which a stack's
// admission refuses.
func (f File) Levels() [][]string {
	waits := make(map[string]int, len(f.Services))
	waiting := make(map[string][]string, len(f.Services))

	for name, service := range f.Services {
		for _, dependency := range service.DependsOn {
			if _, defined := f.Services[dependency]; defined {
				waits[name]++
				waiting[dependency] = append(waiting[dependency], name)
			}
		}
	}

	var level []string
	for name := range f.Services {
		if waits[name] == 0 {
			level = append(level, name)
		}
	}

	var levels [][]string
	for len(level) > 0 {
		slices.Sort(level)
		levels = append(levels, level)

		var next []string
		for _, name := range level {
			for _, dependent := range waiting[name] {
				waits[dependent]--
				if waits[dependent] == 0 {
					next = append(next, dependent)
				}
			}
		}

		level = next
	}

	return levels
}

// Field is what a path in a stack's file is said under: the file's field,
// and the path beneath it.
func Field(path string) string {
	if len(path) == 0 {
		return FileField
	}

	return FileField + "." + path
}

// Parse reads a stack's file, and is it, or everything wrong with it, each
// under the path in the file of what it is wrong with (Field), and the code it
// is refused with.
//
// The file is YAML, a document of one mapping, read strictly into the
// platform's own shape of it (yaml.Decoder.KnownFields): a key it does not
// take is refused under its own path, compose.services.web.healthcheck, be it
// one compose takes that the platform does not carry out, build, healthcheck,
// configs, secrets, profiles, extends, deploy, a top-level version or name or
// an x- key among them, or any other. What it takes:
//
//   - at the top, services, at least one, and the volumes and the networks it
//     declares, each by its name and with nothing beneath it;
//   - for each service, its image, which it needs, and its command and
//     entrypoint, each a list, or a line split into words as a shell would
//     split it; its environment, a mapping of scalars or a list of
//     KEY=VALUE; its ports, [ip:][host:]container[/tcp|udp], ranges and all,
//     or target, published, host_ip and protocol; privileged; restart, no or
//     unless-stopped; depends_on, a list, or a mapping whose one condition is
//     service_started; its volumes, source:target[:ro|rw], or type, which is
//     bind or volume, source, target and read_only; its networks, a list, or
//     a mapping with nothing beneath each; pids_limit, user and runtime; and
//     resources.limits.cpus and resources.limits.memory.
//
// Everything it names is held to being there: a service it depends on, a
// volume it mounts, which the file declares, and a network it joins, which it
// declares, but for the default one. A bind mounts an absolute path of its VM,
// a volume is named, and a name is one docker takes. Nothing is interpolated:
// ${VAR} is what it says. That no services depend on one another in a ring is
// a stack's plan's to say, which says it of each of them (CodeDependsOnCycle).
func Parse(compose string) (File, domain.ValidationErrors) {
	invalid := make(domain.ValidationErrors)

	switch {
	case len(strings.TrimSpace(compose)) == 0:
		invalid[Field("")] = "required_field"

		return File{}, invalid
	case len(compose) > MaxFile:
		invalid[Field("")] = "too_large"

		return File{}, invalid
	}

	root, err := documentOf(compose)
	if err != nil {
		invalid[Field("")] = "invalid_value"

		return File{}, invalid
	}

	r := &reading{invalid: invalid}

	if !r.shaped(root) {
		return File{}, invalid
	}

	var d document

	decoder := yaml.NewDecoder(strings.NewReader(compose))
	decoder.KnownFields(true)

	if err := decoder.Decode(&d); err != nil {
		// what is shaped as the file is read into it: a key it does not take
		// was refused above, under its path.
		invalid[Field("")] = "invalid_value"

		return File{}, invalid
	}

	file := r.file(d)
	if len(invalid) > 0 {
		return File{}, invalid
	}

	return file, nil
}

// documentOf is the one mapping a stack's file is, as YAML has it: one that
// is not YAML, or is more than one document, or is not a mapping, is none.
func documentOf(compose string) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(strings.NewReader(compose))

	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}

	var more yaml.Node
	if err := decoder.Decode(&more); !errors.Is(err, io.EOF) {
		return nil, errors.New("a stack's file is one document")
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil, errors.New("a stack's file is a document")
	}

	mapping := resolved(root.Content[0])
	if mapping.Kind != yaml.MappingNode {
		return nil, errors.New("a stack's file is a mapping")
	}

	return mapping, nil
}

// document is a stack's file as it is decoded, strictly: a key that is no
// field of these is refused (yaml.Decoder.KnownFields), and each value is kept
// as it is written, to be read under its path.
type document struct {
	Services map[string]serviceDocument `yaml:"services"`
	Volumes  map[string]yaml.Node       `yaml:"volumes"`
	Networks map[string]yaml.Node       `yaml:"networks"`
}

// serviceDocument is one service of a stack's file, as it is decoded.
type serviceDocument struct {
	Image       yaml.Node          `yaml:"image"`
	Command     yaml.Node          `yaml:"command"`
	Entrypoint  yaml.Node          `yaml:"entrypoint"`
	Environment yaml.Node          `yaml:"environment"`
	Ports       yaml.Node          `yaml:"ports"`
	Privileged  yaml.Node          `yaml:"privileged"`
	Restart     yaml.Node          `yaml:"restart"`
	DependsOn   yaml.Node          `yaml:"depends_on"`
	Volumes     yaml.Node          `yaml:"volumes"`
	Networks    yaml.Node          `yaml:"networks"`
	PidsLimit   yaml.Node          `yaml:"pids_limit"`
	User        yaml.Node          `yaml:"user"`
	Runtime     yaml.Node          `yaml:"runtime"`
	Resources   *resourcesDocument `yaml:"resources"`
}

// resourcesDocument is what a service of a stack's file is held to, as it is
// decoded: limits, and nothing else.
type resourcesDocument struct {
	Limits *limitsDocument `yaml:"limits"`
}

// limitsDocument is the most a service of a stack's file uses, as it is
// decoded.
type limitsDocument struct {
	CPUs   yaml.Node `yaml:"cpus"`
	Memory yaml.Node `yaml:"memory"`
}

// The keys of each mapping of a stack's file whose keys are fixed, as their
// documents name them.
var (
	fileKeys      = keysOf[document]()
	serviceKeys   = keysOf[serviceDocument]()
	resourcesKeys = keysOf[resourcesDocument]()
	limitsKeys    = keysOf[limitsDocument]()

	// longPortKeys and longVolumeKeys are what a port and a volume of a
	// service say, in their long forms, and conditionKeys what a service
	// depended on is depended on with.
	longPortKeys   = []string{"target", "published", "host_ip", "protocol"}
	longVolumeKeys = []string{"type", "source", "target", "read_only"}
	conditionKeys  = []string{"condition"}
)

// keysOf are the keys a mapping decoded into a T has, as its fields' yaml tags
// name them: what yaml.Decoder.KnownFields holds one to.
func keysOf[T any]() []string {
	t := reflect.TypeFor[T]()

	keys := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		keys = append(keys, name)
	}

	return keys
}

// name is what docker takes as the name of something it makes, which a
// service, a volume and a network of a stack's file are each made part of.
var name = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// memoryWritten is a size as a limit of a service's memory is written: bytes,
// or a number of k, m or g of them, its b said or not.
var memoryWritten = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([kmg]?)b?$`)

// exitedWith reads what a container exited with out of what docker says of it.
var exitedWith = regexp.MustCompile(`^Exited \((-?[0-9]+)\)`)

// ExitCode is what docker says a container exited with, read out of the
// sentence docker puts its state in, and whether it says it.
func ExitCode(status string) (int, bool) {
	code := exitedWith.FindStringSubmatch(status)
	if len(code) != 2 {
		return 0, false
	}

	exited, err := strconv.Atoi(code[1])

	return exited, err == nil
}

// reading is a stack's file being read: what is wrong with it, by the path
// of each thing wrong.
type reading struct {
	invalid domain.ValidationErrors
}

// refuse has the file refused, path being what is wrong, with code: the first
// thing said of a path is what it is refused for.
func (r *reading) refuse(path string, code string) {
	if _, said := r.invalid[Field(path)]; !said {
		r.invalid[Field(path)] = code
	}
}

// shaped reports whether a stack's file is shaped as the platform reads one,
// saying of each mapping whose keys are fixed what is not: a key it does not
// take, or takes once and is given twice, under its own path, and a value
// beneath one that is not shaped as it is to be.
func (r *reading) shaped(root *yaml.Node) bool {
	before := len(r.invalid)

	r.keys(root, "", fileKeys)

	services := entry(root, "services")
	switch {
	case isNull(services) || (services.Kind == yaml.MappingNode && len(services.Content) == 0):
		r.refuse("services", "required_field")
	case services.Kind != yaml.MappingNode:
		r.refuse("services", "invalid_value")
	default:
		r.each(services, "services", func(_ string, path string, service *yaml.Node) {
			if service.Kind != yaml.MappingNode {
				r.refuse(path, "invalid_value")

				return
			}

			r.keys(service, path, serviceKeys)

			resources := entry(service, "resources")
			if !r.mappingOrNull(resources, field(path, "resources")) {
				return
			}

			r.keys(resources, field(path, "resources"), resourcesKeys)

			limits := entry(resources, "limits")
			if r.mappingOrNull(limits, field(path, "resources", "limits")) {
				r.keys(limits, field(path, "resources", "limits"), limitsKeys)
			}
		})
	}

	for _, declared := range []string{"volumes", "networks"} {
		if top := entry(root, declared); r.mappingOrNull(top, declared) {
			r.each(top, declared, func(_ string, path string, value *yaml.Node) {
				if r.mappingOrNull(value, path) {
					r.keys(value, path, nil)
				}
			})
		}
	}

	return len(r.invalid) == before
}

// keys says of a mapping at path each key it does not take, and each it is
// given twice, under its own path. A mapping that is not there takes any.
func (r *reading) keys(mapping *yaml.Node, path string, takes []string) {
	mapping = resolved(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}

	seen := make(map[string]bool)

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := keyOf(mapping.Content[i])

		switch {
		case !slices.Contains(takes, key):
			r.refuse(field(path, key), CodeUnsupportedKey)
		case seen[key]:
			r.refuse(field(path, key), CodeDuplicateKey)
		}

		seen[key] = true
	}
}

// unique says of a mapping at path each key it is given twice, under its own
// path: one whose keys are anything of a service's, its environment's or what
// it depends on.
func (r *reading) unique(mapping *yaml.Node, path string) {
	seen := make(map[string]bool)

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := keyOf(mapping.Content[i])
		if seen[key] {
			r.refuse(field(path, key), CodeDuplicateKey)
		}

		seen[key] = true
	}
}

// each has what of each entry of a mapping at path, by its name, which has to
// be a name docker takes and be given once, under the entry's path.
func (r *reading) each(mapping *yaml.Node, path string, what func(key string, path string, value *yaml.Node)) {
	seen := make(map[string]bool)

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := keyOf(mapping.Content[i])
		at := field(path, key)

		switch {
		case !name.MatchString(key):
			r.refuse(at, "invalid_name")
		case seen[key]:
			r.refuse(at, CodeDuplicateKey)
		default:
			what(key, at, resolved(mapping.Content[i+1]))
		}

		seen[key] = true
	}
}

// mappingOrNull reports whether a value at path is a mapping, saying it is
// not when it is anything else but nothing: what has nothing beneath it is
// shaped as anything is.
func (r *reading) mappingOrNull(value *yaml.Node, path string) bool {
	switch {
	case isNull(value):
		return false
	case resolved(value).Kind != yaml.MappingNode:
		r.refuse(path, "invalid_value")

		return false
	}

	return true
}

// file is a stack's file as decoded read, each value under its path, and
// held to what it names being there.
func (r *reading) file(d document) File {
	f := File{
		Services: make(map[string]Service, len(d.Services)),
		Volumes:  slices.Sorted(maps.Keys(d.Volumes)),
		Networks: slices.Sorted(maps.Keys(d.Networks)),
	}

	for _, name := range slices.Sorted(maps.Keys(d.Services)) {
		f.Services[name] = r.service(field("services", name), d.Services[name])
	}

	// what each names is there.
	for _, name := range slices.Sorted(maps.Keys(f.Services)) {
		path := field("services", name)
		service := f.Services[name]

		for _, dependency := range service.DependsOn {
			if _, defined := f.Services[dependency]; !defined {
				r.refuse(field(path, "depends_on", dependency), CodeUndefinedService)
			}
		}

		for _, network := range service.Networks {
			if network != DefaultNetwork && !slices.Contains(f.Networks, network) {
				r.refuse(field(path, "networks", network), CodeUndefinedNetwork)
			}
		}

		for i, m := range service.Volumes {
			if m.Type == MountVolume && !slices.Contains(f.Volumes, m.Source) {
				r.refuse(indexed(field(path, "volumes"), i), CodeUndefinedVolume)
			}
		}
	}

	return f
}

// service is one service of a stack's file, read under path.
func (r *reading) service(path string, d serviceDocument) Service {
	s := Service{
		Image:       r.image(&d.Image, field(path, "image")),
		Command:     r.words(&d.Command, field(path, "command")),
		Entrypoint:  r.words(&d.Entrypoint, field(path, "entrypoint")),
		Environment: r.environment(&d.Environment, field(path, "environment")),
		Ports:       r.ports(&d.Ports, field(path, "ports")),
		Privileged:  r.boolean(&d.Privileged, field(path, "privileged")),
		Restart:     r.restart(&d.Restart, field(path, "restart")),
		DependsOn:   r.dependsOn(&d.DependsOn, field(path, "depends_on")),
		Volumes:     r.volumes(&d.Volumes, field(path, "volumes")),
		Networks:    r.networks(&d.Networks, field(path, "networks")),
		PidsLimit:   r.pidsLimit(&d.PidsLimit, field(path, "pids_limit")),
		User:        r.text(&d.User, field(path, "user")),
		Runtime:     r.text(&d.Runtime, field(path, "runtime")),
	}

	if d.Resources != nil && d.Resources.Limits != nil {
		limits := field(path, "resources", "limits")

		s.CPUs = r.cpus(&d.Resources.Limits.CPUs, field(limits, "cpus"))
		s.Memory = r.memory(&d.Resources.Limits.Memory, field(limits, "memory"))
	}

	return s
}

// image is what a service runs: a reference with no blank in it, which it
// has to say.
func (r *reading) image(n *yaml.Node, path string) string {
	if isNull(n) {
		r.refuse(path, "required_field")

		return ""
	}

	image, ok := scalar(n)

	switch {
	case !ok:
		r.refuse(path, "invalid_value")
	case len(strings.TrimSpace(image)) == 0:
		r.refuse(path, "required_field")
	case strings.ContainsFunc(image, isBlank):
		r.refuse(path, "invalid_image")
	}

	return image
}

// words are a command's or an entrypoint's words: each of a list, or a line
// split as a shell would split it, none when it says nothing.
func (r *reading) words(n *yaml.Node, path string) []string {
	if isNull(n) {
		return nil
	}

	if line, ok := scalar(n); ok {
		split, ok := Words(line)
		if !ok {
			r.refuse(path, "invalid_value")
		}

		return split
	}

	return r.list(n, path)
}

// list is each scalar of a sequence, as it is written.
func (r *reading) list(n *yaml.Node, path string) []string {
	n = resolved(n)
	if n.Kind != yaml.SequenceNode {
		r.refuse(path, "invalid_value")

		return nil
	}

	values := make([]string, 0, len(n.Content))

	for i, item := range n.Content {
		value, ok := scalar(item)
		if !ok || isNull(item) {
			r.refuse(indexed(path, i), "invalid_value")

			continue
		}

		values = append(values, value)
	}

	return values
}

// environment is each variable a service is given, as KEY=value, in the
// order it says them: a mapping of each to a scalar, nothing being nothing,
// or a list of KEY=VALUE. A key is something, and has no = in it.
func (r *reading) environment(n *yaml.Node, path string) []string {
	if isNull(n) {
		return nil
	}

	n = resolved(n)

	var variables []string

	switch n.Kind {
	case yaml.MappingNode:
		r.unique(n, path)

		for i := 0; i+1 < len(n.Content); i += 2 {
			key := keyOf(n.Content[i])

			value, ok := scalar(n.Content[i+1])
			if isNull(n.Content[i+1]) {
				value, ok = "", true
			}

			if !ok || len(key) == 0 || strings.Contains(key, "=") {
				r.refuse(field(path, key), "invalid_value")

				continue
			}

			variables = append(variables, key+"="+value)
		}

	case yaml.SequenceNode:
		for i, variable := range r.list(n, path) {
			if key, _, ok := strings.Cut(variable, "="); !ok || len(key) == 0 {
				r.refuse(indexed(path, i), "invalid_value")

				continue
			}

			variables = append(variables, variable)
		}

	default:
		r.refuse(path, "invalid_value")
	}

	return variables
}

// ports are each port a service publishes, a range being each of its ports:
// [ip:][host:]container[/tcp|udp] or target, published, host_ip and protocol.
func (r *reading) ports(n *yaml.Node, path string) []Port {
	if isNull(n) {
		return nil
	}

	n = resolved(n)
	if n.Kind != yaml.SequenceNode {
		r.refuse(path, "invalid_value")

		return nil
	}

	var ports []Port

	for i, item := range n.Content {
		at := indexed(path, i)
		item = resolved(item)

		if item.Kind == yaml.MappingNode {
			ports = append(ports, r.longPort(item, at)...)

			continue
		}

		written, ok := scalar(item)
		if !ok || isNull(item) {
			r.refuse(at, CodeInvalidPortMapping)

			continue
		}

		published, code := shortPort(written)
		if len(code) > 0 {
			r.refuse(at, code)

			continue
		}

		ports = append(ports, published...)
	}

	return ports
}

// longPort is a port a service publishes, in its long form: target,
// published, host_ip and protocol, the target one port and published one or
// none.
func (r *reading) longPort(n *yaml.Node, path string) []Port {
	r.keys(n, path, longPortKeys)

	p := Port{Protocol: ProtocolTCP}

	target, ok := portOf(entry(n, "target"))
	if !ok {
		r.refuse(field(path, "target"), CodeInvalidPortMapping)
	}

	p.Container = target

	if published := entry(n, "published"); !isNull(published) {
		if p.Host, ok = portOf(published); !ok {
			r.refuse(field(path, "published"), CodeInvalidPortMapping)
		}
	}

	if hostIP := entry(n, "host_ip"); !isNull(hostIP) {
		if p.HostIP, ok = scalar(hostIP); !ok || !isIP(p.HostIP) {
			r.refuse(field(path, "host_ip"), "invalid_ip")
		}
	}

	if protocol := entry(n, "protocol"); !isNull(protocol) {
		if p.Protocol, ok = scalar(protocol); !ok || (p.Protocol != ProtocolTCP && p.Protocol != ProtocolUDP) {
			r.refuse(field(path, "protocol"), "invalid_protocol")
		}
	}

	return []Port{p}
}

// shortPort is each port [ip:][host:]container[/tcp|udp] publishes, ranges of
// as many ports on either side being each of them, or the code it is refused
// with: an address in brackets when it is an IPv6 one, a host that says no
// port being one docker picks, and a range of container ports with none on
// the host each with one docker picks.
func shortPort(written string) ([]Port, string) {
	mapping, protocol, protocolled := strings.Cut(written, "/")
	if !protocolled {
		protocol = ProtocolTCP
	}

	if protocol != ProtocolTCP && protocol != ProtocolUDP {
		return nil, "invalid_protocol"
	}

	var hostIP string

	if strings.HasPrefix(mapping, "[") {
		closing := strings.Index(mapping, "]:")
		if closing < 0 {
			return nil, CodeInvalidPortMapping
		}

		hostIP, mapping = mapping[1:closing], mapping[closing+2:]

		switch {
		case !isIP(hostIP):
			return nil, "invalid_ip"
		case !strings.Contains(mapping, ":"):
			return nil, CodeInvalidPortMapping
		}
	}

	parts := strings.Split(mapping, ":")

	var host, container string

	switch len(parts) {
	case 1:
		container = parts[0]
	case 2:
		host, container = parts[0], parts[1]
	case 3:
		if len(hostIP) > 0 {
			return nil, CodeInvalidPortMapping
		}

		hostIP, host, container = parts[0], parts[1], parts[2]

		if !isIP(hostIP) {
			return nil, "invalid_ip"
		}
	default:
		return nil, CodeInvalidPortMapping
	}

	containers, ok := portRange(container)
	if !ok {
		return nil, CodeInvalidPortMapping
	}

	var hosts []port.Port

	if len(host) > 0 {
		if hosts, ok = portRange(host); !ok || (len(hosts) != len(containers)) {
			return nil, CodeInvalidPortMapping
		}
	}

	if len(containers) > maxRange {
		return nil, "too_large"
	}

	ports := make([]Port, len(containers))
	for i, c := range containers {
		ports[i] = Port{Container: c, HostIP: hostIP, Protocol: protocol}

		if hosts != nil {
			ports[i].Host = hosts[i]
		}
	}

	return ports, ""
}

// portRange is each port a port, or a range of them first-last, says, and
// whether it says any: a port is from 1 to 65535.
func portRange(written string) ([]port.Port, bool) {
	first, last, ranged := strings.Cut(written, "-")
	if !ranged {
		last = first
	}

	from, fromOK := parsePort(first)
	to, toOK := parsePort(last)

	if !fromOK || !toOK || to < from {
		return nil, false
	}

	ports := make([]port.Port, 0, to-from+1)
	for p := from; p <= to && len(ports) <= maxRange; p++ {
		ports = append(ports, p)
	}

	return ports, true
}

// portOf is the one port a node says, and whether it says one.
func portOf(n *yaml.Node) (port.Port, bool) {
	written, ok := scalar(n)
	if !ok || isNull(n) {
		return 0, false
	}

	return parsePort(written)
}

// parsePort is the port written says, and whether it is one: a number from 1
// to 65535.
func parsePort(written string) (port.Port, bool) {
	parsed, err := strconv.ParseUint(written, 10, 16)
	if err != nil || parsed == 0 {
		return 0, false
	}

	return port.Port(parsed), true
}

// boolean is true or false, false when it says nothing.
func (r *reading) boolean(n *yaml.Node, path string) bool {
	if isNull(n) {
		return false
	}

	written, ok := scalar(n)

	switch {
	case ok && written == "true":
		return true
	case ok && written == "false":
		return false
	}

	r.refuse(path, "invalid_value")

	return false
}

// restart is no or unless-stopped, no when it says nothing.
func (r *reading) restart(n *yaml.Node, path string) string {
	if isNull(n) {
		return RestartNo
	}

	written, ok := scalar(n)
	if !ok || (written != RestartNo && written != RestartUnlessStopped) {
		r.refuse(path, CodeInvalidRestart)

		return RestartNo
	}

	return written
}

// dependsOn are the services a service is started after, in the order of
// their names, each once: a list of them, or a mapping of each to the one
// condition it is waited on with, or to nothing, which is that one.
func (r *reading) dependsOn(n *yaml.Node, path string) []string {
	if isNull(n) {
		return nil
	}

	n = resolved(n)

	var services []string

	switch n.Kind {
	case yaml.SequenceNode:
		for i, service := range r.list(n, path) {
			if slices.Contains(services, service) {
				r.refuse(indexed(path, i), CodeDuplicateKey)

				continue
			}

			services = append(services, service)
		}

	case yaml.MappingNode:
		r.unique(n, path)

		for i := 0; i+1 < len(n.Content); i += 2 {
			service := keyOf(n.Content[i])
			at := field(path, service)

			if condition := resolved(n.Content[i+1]); !isNull(condition) {
				if !r.mappingOrNull(condition, at) {
					continue
				}

				r.keys(condition, at, conditionKeys)

				if waited, ok := scalar(entry(condition, "condition")); !isNull(entry(condition, "condition")) && (!ok || waited != ConditionStarted) {
					r.refuse(field(at, "condition"), CodeInvalidCondition)
				}
			}

			if !slices.Contains(services, service) {
				services = append(services, service)
			}
		}

	default:
		r.refuse(path, "invalid_value")
	}

	slices.Sort(services)

	return services
}

// volumes are what a service mounts: source:target[:ro|rw], or type, source,
// target and read_only, a bind of an absolute path of its VM, or a volume
// by its name, which its file declares.
func (r *reading) volumes(n *yaml.Node, path string) []Mount {
	if isNull(n) {
		return nil
	}

	n = resolved(n)
	if n.Kind != yaml.SequenceNode {
		r.refuse(path, "invalid_value")

		return nil
	}

	var mounts []Mount

	for i, item := range n.Content {
		at := indexed(path, i)
		item = resolved(item)

		if item.Kind == yaml.MappingNode {
			if m, read := r.longVolume(item, at); read {
				mounts = append(mounts, m)
			}

			continue
		}

		written, ok := scalar(item)
		if !ok || isNull(item) {
			r.refuse(at, "invalid_mount")

			continue
		}

		m, code := shortVolume(written)
		if len(code) > 0 {
			r.refuse(at, code)

			continue
		}

		mounts = append(mounts, m)
	}

	return mounts
}

// longVolume is what a service mounts, in its long form, and whether it can
// be read as a mount, saying under path what is wrong with it otherwise.
func (r *reading) longVolume(n *yaml.Node, path string) (Mount, bool) {
	before := len(r.invalid)

	r.keys(n, path, longVolumeKeys)

	if len(r.invalid) > before {
		return Mount{}, false
	}

	refused := func(code string) (Mount, bool) {
		r.refuse(path, code)

		return Mount{}, false
	}

	var m Mount

	written, ok := scalar(entry(n, "type"))
	if !ok || (written != MountBind && written != MountVolume) {
		return refused(CodeInvalidVolumeType)
	}

	m.Type = written

	if source := entry(n, "source"); !isNull(source) {
		if m.Source, ok = scalar(source); !ok {
			return refused("invalid_mount")
		}
	}

	if m.Target, ok = scalar(entry(n, "target")); !ok || isNull(entry(n, "target")) {
		return refused("invalid_mount")
	}

	if readOnly := entry(n, "read_only"); !isNull(readOnly) {
		written, ok := scalar(readOnly)
		if !ok || (written != "true" && written != "false") {
			return refused("invalid_mount")
		}

		m.ReadOnly = written == "true"
	}

	if code := mountCode(m); len(code) > 0 {
		return refused(code)
	}

	return m, true
}

// shortVolume is what source:target[:ro|rw] mounts, or the code it is refused
// with: a source that is a path is a bind of it, and any other the volume it
// names; and a target alone is a volume with no name.
func shortVolume(written string) (Mount, string) {
	parts := strings.Split(written, ":")

	var m Mount

	switch len(parts) {
	case 1:
		return Mount{}, CodeAnonymousVolume
	case 3:
		switch parts[2] {
		case "ro":
			m.ReadOnly = true
		case "rw":
		default:
			return Mount{}, "invalid_mount"
		}
	case 2:
	default:
		return Mount{}, "invalid_mount"
	}

	m.Source, m.Target = parts[0], parts[1]

	m.Type = MountVolume
	if strings.HasPrefix(m.Source, "/") || strings.HasPrefix(m.Source, ".") || strings.HasPrefix(m.Source, "~") || strings.Contains(m.Source, "/") {
		m.Type = MountBind
	}

	return m, mountCode(m)
}

// mountCode is what a mount is refused with, or nothing: a bind of a path
// that is not absolute, a volume with no name or a name docker does not take,
// and a target that is not an absolute path.
func mountCode(m Mount) string {
	switch {
	case len(m.Source) == 0 && m.Type == MountVolume:
		return CodeAnonymousVolume
	case m.Type == MountBind && !strings.HasPrefix(m.Source, "/"):
		return CodeRelativeBind
	case m.Type == MountVolume && !name.MatchString(m.Source):
		return "invalid_name"
	case !strings.HasPrefix(m.Target, "/"):
		return "invalid_mount"
	}

	return ""
}

// networks are the networks a service joins, in the order of their names,
// each once: a list, or a mapping of each to nothing; the default one when it
// names none.
func (r *reading) networks(n *yaml.Node, path string) []string {
	var networks []string

	if !isNull(n) {
		n = resolved(n)

		switch n.Kind {
		case yaml.SequenceNode:
			for i, network := range r.list(n, path) {
				switch {
				case !name.MatchString(network):
					r.refuse(indexed(path, i), "invalid_name")
				case slices.Contains(networks, network):
					r.refuse(indexed(path, i), CodeDuplicateKey)
				default:
					networks = append(networks, network)
				}
			}

		case yaml.MappingNode:
			r.each(n, path, func(network string, at string, value *yaml.Node) {
				if r.mappingOrNull(value, at) {
					r.keys(value, at, nil)
				}

				networks = append(networks, network)
			})

		default:
			r.refuse(path, "invalid_value")
		}
	}

	if len(networks) == 0 {
		networks = []string{DefaultNetwork}
	}

	slices.Sort(networks)

	return networks
}

// pidsLimit is the most processes a service runs at once, a whole number of
// them, nothing when it says nothing.
func (r *reading) pidsLimit(n *yaml.Node, path string) int64 {
	if isNull(n) {
		return 0
	}

	written, ok := scalar(n)
	limit, err := strconv.ParseInt(written, 10, 64)

	if !ok || err != nil || limit < 0 {
		r.refuse(path, "invalid_value")

		return 0
	}

	return limit
}

// text is what a scalar says, as it is written, nothing when it says
// nothing.
func (r *reading) text(n *yaml.Node, path string) string {
	if isNull(n) {
		return ""
	}

	written, ok := scalar(n)
	if !ok {
		r.refuse(path, "invalid_value")
	}

	return written
}

// cpus is a number of cores above nothing, nothing when it says nothing.
func (r *reading) cpus(n *yaml.Node, path string) float64 {
	if isNull(n) {
		return 0
	}

	written, ok := scalar(n)
	cpus, err := strconv.ParseFloat(written, 64)

	if !ok || err != nil || math.IsNaN(cpus) || math.IsInf(cpus, 0) || cpus <= 0 {
		r.refuse(path, CodeInvalidCPUs)

		return 0
	}

	return cpus
}

// memory is a number of bytes above nothing, or of k, m or g of them,
// nothing when it says nothing.
func (r *reading) memory(n *yaml.Node, path string) uint64 {
	if isNull(n) {
		return 0
	}

	written, ok := scalar(n)
	bytes, valid := Bytes(written)

	if !ok || !valid {
		r.refuse(path, CodeInvalidMemory)

		return 0
	}

	return bytes
}

// Bytes is how many bytes a size says, and whether it says one above
// nothing: bytes, or a number of k, m or g of them, each 1024 of the one
// before, its b said or not.
func Bytes(written string) (uint64, bool) {
	size := memoryWritten.FindStringSubmatch(strings.TrimSpace(written))
	if size == nil {
		return 0, false
	}

	var unit float64

	switch strings.ToLower(size[2]) {
	case "":
		unit = 1
	case "k":
		unit = 1 << 10
	case "m":
		unit = 1 << 20
	case "g":
		unit = 1 << 30
	}

	number, err := strconv.ParseFloat(size[1], 64)
	if err != nil {
		return 0, false
	}

	bytes := number * unit

	// a fraction of a byte is none.
	if bytes < 1 || bytes != math.Trunc(bytes) || bytes >= math.MaxUint64 {
		return 0, false
	}

	return uint64(bytes), true
}

// Words are the words a shell would split a line into, without running one,
// and whether it can be split: separated by blanks; a word quoted whole in
// single quotes; or in double quotes, in which a backslash keeps a quote, a
// backslash, a dollar or a backquote after it as it is; and a backslash
// outside quotes keeping whatever is after it. Nothing is expanded: a dollar
// is a dollar. A quote left open, and a backslash at the end, split it not at
// all.
func Words(line string) ([]string, bool) {
	runes := []rune(line)

	var (
		words []string
		word  bytes.Buffer
		begun bool
	)

	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; {
		case c == '\\':
			if i+1 == len(runes) {
				return nil, false
			}

			i++

			// a backslash before a line's end joins it to the next.
			if runes[i] != '\n' {
				word.WriteRune(runes[i])
				begun = true
			}

		case c == '\'':
			closing := slices.Index(runes[i+1:], '\'')
			if closing < 0 {
				return nil, false
			}

			word.WriteString(string(runes[i+1 : i+1+closing]))
			begun = true
			i += closing + 1

		case c == '"':
			i++

			for ; i < len(runes) && runes[i] != '"'; i++ {
				if runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune("$`\"\\\n", runes[i+1]) {
					i++

					if runes[i] == '\n' {
						continue
					}
				}

				word.WriteRune(runes[i])
			}

			if i == len(runes) {
				return nil, false
			}

			begun = true

		case c == ' ' || c == '\t' || c == '\n':
			if begun {
				words = append(words, word.String())
				word.Reset()
				begun = false
			}

		default:
			word.WriteRune(c)
			begun = true
		}
	}

	if begun {
		words = append(words, word.String())
	}

	return words, true
}

// entry is the value a mapping gives key, or nothing when it gives none.
func entry(mapping *yaml.Node, key string) *yaml.Node {
	mapping = resolved(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if keyOf(mapping.Content[i]) == key {
			return resolved(mapping.Content[i+1])
		}
	}

	return nil
}

// keyOf is a mapping's key as it is written.
func keyOf(key *yaml.Node) string {
	return resolved(key).Value
}

// resolved is what n is: what an alias stands for, when it is one.
func resolved(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}

	return n
}

// isNull reports whether n says nothing: there is none, or it is null.
func isNull(n *yaml.Node) bool {
	n = resolved(n)

	return n == nil || n.Kind == 0 || (n.Kind == yaml.ScalarNode && n.Tag == "!!null")
}

// scalar is what a scalar says, as it is written, and whether n is one.
func scalar(n *yaml.Node) (string, bool) {
	n = resolved(n)
	if n == nil || n.Kind != yaml.ScalarNode {
		return "", false
	}

	return n.Value, true
}

// isBlank reports whether c is blank.
func isBlank(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// isIP reports whether address is one a port can be published on: an IPv4 or
// an IPv6 address, with no zone.
func isIP(address string) bool {
	parsed, err := netip.ParseAddr(address)

	return err == nil && len(parsed.Zone()) == 0
}

// field is the path of keys beneath path.
func field(path string, keys ...string) string {
	for _, key := range keys {
		if len(path) == 0 {
			path = key
		} else {
			path += "." + key
		}
	}

	return path
}

// indexed is the path of the item at i of a sequence at path.
func indexed(path string, i int) string {
	return fmt.Sprintf("%s[%d]", path, i)
}
