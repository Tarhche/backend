package stack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net/netip"
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

// The most a stack's file may say of each thing, so that what reading it, and
// planning and carrying out what it says, takes is bounded by these, as what
// it is is by MaxFile: it says nothing twice, an anchor or an alias being
// refused (CodeUnsupportedAlias), and a range of ports is counted by its size
// before any port of it is.
const (
	// MaxServices, MaxNetworks and MaxVolumes are the most services,
	// networks and volumes it declares, each refused as too_large beyond
	// them, under its services, its networks or its volumes.
	MaxServices = 100
	MaxNetworks = 100
	MaxVolumes  = 100

	// MaxPorts is the most ports its services publish, all of them
	// together, each port of a range one: the port that would take them past
	// it is refused as too_large, and no port after it is read.
	MaxPorts = 1000

	// MaxReferences is the most its services name of what it declares, all
	// of them together, which is what planning its graph takes: each network
	// a service joins, the default one once for each service that names
	// none, each volume it mounts by its name, and each service it depends
	// on. The reference that would take them past it is refused as
	// too_large, under its own path, and no reference after it is read.
	MaxReferences = 1000

	// MaxNameLength is the most characters the name of a service, a network
	// or a volume has, and so whatever names one: a service's name is what
	// its neighbours reach it by, which DNS holds to as many. A longer one is
	// refused as too_large, once, under its own path, and nothing beneath it
	// is read. It is counted in bytes, each of which is a character of a name
	// docker takes.
	MaxNameLength = 63

	// MaxDepth is how many levels beneath its top anything in it is at most:
	// a key of its own mapping, and the value of one, is a level beneath it,
	// and a key, a value or an item beneath that one is a level more. What is
	// deeper is refused as too_deep, under the path of the first of it in
	// what holds it, and nothing beneath that is read.
	MaxDepth = 16

	// MaxInvalid is the most of what is wrong with it that is said: the
	// first of it, as the file is read, in the order the file says things.
	// Nothing more of it is read once that much is said.
	MaxInvalid = 100
)

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
	// any other; and one that is not a string as YAML reads it, a number, a
	// boolean, nothing, a merge or what a tag says is anything else, which
	// is quoted to be taken as a string.
	CodeUnsupportedKey = "unsupported_key"

	// CodeDuplicateKey is a key a mapping of the file says twice.
	CodeDuplicateKey = "duplicate_key"

	// CodeUnsupportedAlias is an anchor (&name) or an alias (*name), which
	// the file does not take: what it says, it says once, so that reading it
	// is never more than what it is.
	CodeUnsupportedAlias = "unsupported_alias"

	// CodeTooDeep is what is nested in the file deeper than MaxDepth.
	CodeTooDeep = "too_deep"

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
// platform's own shape of it: a key it does not take, which is no field of
// that shape, is refused under its own path before anything is read into it,
// compose.services.web.healthcheck, be it one compose takes that the platform
// does not carry out, build, healthcheck, configs, secrets, profiles, extends,
// deploy, a top-level version or name or an x- key among them, or any other;
// and so is a key, anywhere in it, that is not a string as YAML reads it, a
// number, a boolean, nothing, a merge or anything a tag says it is but a
// string, which is quoted to be taken as one. Once nothing is wrong with it,
// it is decoded strictly into that shape too (yaml.Decoder.KnownFields),
// which refuses nothing it was not refused for already: what that refuses all
// the same is a mistake of the reading's, which refuses the file under its
// own path, compose, and is logged. What it takes:
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
// declares, but for the default one, each by its name as the file writes it,
// which is the name of what it declares, too. A bind mounts an absolute path
// of its VM, a volume is named, and a name, of a service, a volume or a
// network, and whatever names one, is one docker takes, of MaxNameLength
// characters at most. Nothing is interpolated: ${VAR} is what it says. That
// no services depend on one another in a ring is a stack's plan's to say,
// which says it of each of them (CodeDependsOnCycle).
//
// What reading it takes is bounded by what it is: an anchor or an alias,
// anywhere in it, is refused under its path (CodeUnsupportedAlias) before
// anything else of it is read, so that nothing in it is read twice, and so is
// a key that is not a string, and what is nested in it deeper than MaxDepth
// (CodeTooDeep); it declares MaxServices services, MaxNetworks networks and
// MaxVolumes volumes at most, and its services publish MaxPorts ports at
// most, a range counted by its size before any port of it is, and name
// MaxReferences of what it declares at most, which planning its graph takes
// as long as, each counted as it is read; a name refused is said once, and
// nothing beneath it is read; where anything is in it is
// written out as a path only once it is refused; and MaxInvalid of what is
// wrong with it is said at most, the first of it, as it is read, in the order
// it says things, and nothing more of it is read once that much is. A file
// that is taken is read twice, into nodes and decoded strictly, and one that
// is refused once, into nodes.
func Parse(compose string) (File, domain.ValidationErrors) {
	return parse(compose, strictly)
}

// parse is Parse, decoding a file nothing is wrong with strictly with
// decode.
func parse(compose string, decode func(compose string) error) (File, domain.ValidationErrors) {
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

	r := &reading{invalid: invalid, declared: declarations{
		nodes:    make(map[string]*yaml.Node),
		volumes:  make(map[string]bool),
		networks: make(map[string]bool),
	}}

	if !r.sound(root) || !r.shaped(root) {
		return File{}, invalid
	}

	file := r.file()
	if len(invalid) > 0 {
		return File{}, invalid
	}

	// a key shaped refused under its path, as yaml.Decoder.KnownFields would
	// refuse it, and so a file decode refuses all the same is one shaped took
	// by its mistake: it is refused, as what the platform does not read, and
	// the mistake is said.
	if err := decode(compose); err != nil {
		slog.Error("a stack's file the platform's reading took was refused by its strict decode, which the reading is to refuse first, under its path", "error", err)

		invalid[Field("")] = "invalid_value"

		return File{}, invalid
	}

	return file, nil
}

// strictly is what decoding a stack's file into the platform's own shape of
// it (document) fails with, holding each mapping of it whose keys are fixed
// to its fields' (yaml.Decoder.KnownFields): nothing, for a file shaped as the
// platform reads one (reading.shaped). It is read again from its text, since
// what is read into nodes is decoded with no such check.
func strictly(compose string) error {
	decoder := yaml.NewDecoder(strings.NewReader(compose))
	decoder.KnownFields(true)

	var d document

	return decoder.Decode(&d)
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

	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, errors.New("a stack's file is a mapping")
	}

	return mapping, nil
}

// document is a stack's file as it is decoded, strictly (strictly): a key
// that is no field of these is refused (yaml.Decoder.KnownFields), and each
// value is kept as a node, which takes whatever is written. A file is read off
// the nodes reading.shaped walked, which refused such a key under its own
// path before: what it is decoded into is the backstop of that, and is not
// read.
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

// The keys of each mapping of a stack's file whose keys are fixed, which
// reading.shaped holds it to under its path: those its document has fields
// of, written out rather than read off the fields, so that a field a document
// is given is refused until the reading takes it, and each of them paired
// with the document whose fields they are (TestShapes).
var (
	fileKeys      = []string{"services", "volumes", "networks"}
	serviceKeys   = []string{"image", "command", "entrypoint", "environment", "ports", "privileged", "restart", "depends_on", "volumes", "networks", "pids_limit", "user", "runtime", "resources"}
	resourcesKeys = []string{"limits"}
	limitsKeys    = []string{"cpus", "memory"}

	// longPortKeys and longVolumeKeys are what a port and a volume of a
	// service say, in their long forms, and conditionKeys what a service
	// depended on is depended on with.
	longPortKeys   = []string{"target", "published", "host_ip", "protocol"}
	longVolumeKeys = []string{"type", "source", "target", "read_only"}
	conditionKeys  = []string{"condition"}
)

// name is what docker takes as the name of something it makes, which a
// service, a volume and a network of a stack's file are each made part of.
var name = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// nameCode is what a name of a service, a volume or a network of a stack's
// file, or what names one, is refused with, or nothing: one longer than
// MaxNameLength, too_large, before anything else of it is looked at, and one
// docker does not take, invalid_name.
func nameCode(written string) string {
	switch {
	case len(written) > MaxNameLength:
		return "too_large"
	case !name.MatchString(written):
		return "invalid_name"
	}

	return ""
}

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
// of each thing wrong, and what is read of it so far that the rest is held
// to.
type reading struct {
	invalid domain.ValidationErrors

	// declared are the services, the volumes and the networks the file
	// declares, as its walk read them (shaped), which what its services name
	// is held to being.
	declared declarations

	// published are how many ports its services read so far publish, each
	// port of a range one, and capped whether a port would have taken them
	// past MaxPorts, which no port after it is read once it would.
	published int
	capped    bool

	// references are how many references its services read so far make
	// (MaxReferences), and referencesCapped whether one would have taken
	// them past it, which no reference after it is read once it would.
	references       int
	referencesCapped bool
}

// declarations are the services, the volumes and the networks a stack's file
// declares, by their names as it writes them, which are what its walk read:
// its services in the order it says them, and each one's node as it is
// written.
type declarations struct {
	services []string
	nodes    map[string]*yaml.Node
	volumes  map[string]bool
	networks map[string]bool
}

// refuse has the file refused, path being what is wrong, with code: the first
// thing said of a path is what it is refused for, and nothing more is said
// once MaxInvalid things are.
func (r *reading) refuse(path string, code string) {
	if r.full() {
		return
	}

	under := Field(path)

	if _, said := r.invalid[under]; !said {
		r.invalid[under] = code
	}
}

// full reports whether as much of what is wrong with the file is said as is
// ever said (MaxInvalid): nothing more of it is read once it is.
func (r *reading) full() bool {
	return len(r.invalid) >= MaxInvalid
}

// sound reports whether a stack's file, root, is sound to be read: nothing
// of it an anchor or an alias, which would have what it stands for read
// again wherever it is, every key of it a string (stringKey), and nothing of
// it deeper than MaxDepth. One that is not is walked as it is written
// (walker), which says of each node that is not, under its path, what it is
// refused for: an anchor or an alias (CodeUnsupportedAlias), a key that is no
// string (CodeUnsupportedKey), and the first of what is too deep in what
// holds it (CodeTooDeep). What an alias stands for is never followed, and a
// file that is sound is looked over once, allocating nothing.
func (r *reading) sound(root *yaml.Node) bool {
	if isSound(root, 0) {
		return true
	}

	w := walker{reading: r}
	w.walk(root, 0)

	return false
}

// isSound reports whether n, depth levels beneath the top of a stack's file,
// and everything beneath it, is sound to be read (reading.sound), allocating
// nothing, and looking at nothing deeper than MaxDepth.
func isSound(n *yaml.Node, depth int) bool {
	switch {
	case aliasing(n):
		return false
	case len(n.Content) > 0 && depth >= MaxDepth:
		return false
	}

	for i, child := range n.Content {
		if n.Kind == yaml.MappingNode && i%2 == 0 && !stringKey(child) {
			return false
		}

		if !isSound(child, depth+1) {
			return false
		}
	}

	return true
}

// aliasing reports whether n is an anchor or an alias.
func aliasing(n *yaml.Node) bool {
	return n.Kind == yaml.AliasNode || len(n.Anchor) > 0
}

// stringKey reports whether key, a key of a mapping, is a string as YAML
// reads it: a scalar it reads as one, written plainly or quoted, and tagged
// as nothing else. A number, a boolean, nothing, a merge, a date, and what a
// tag says is anything but a string, binary say, are not, and are quoted to
// be taken as one.
func stringKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.ShortTag() == "!!str"
}

// walker walks a stack's file that is not sound to be read as it is written,
// saying of each node that is not, under its path, what it is refused for
// (reading.sound). Where it is is kept as the steps of its path, which are
// written out as one only for a node that is refused, so that walking a file
// takes as much as the file is, and as much again as each path said is; and
// it stops once as much is said of the file as ever is (MaxInvalid).
type walker struct {
	*reading

	steps []step
}

// step is one step of a path in a stack's file: a key, or the index of an
// item of a sequence.
type step struct {
	key   string
	index int
	item  bool
}

// walk says what of n, depth levels beneath the top of the file and at the
// walker's path, is not sound to be read: n being an anchor or an alias, and
// then what is beneath it.
func (w *walker) walk(n *yaml.Node, depth int) {
	if aliasing(n) {
		w.refuse(w.path(), CodeUnsupportedAlias)
	}

	w.beneath(n, depth)
}

// beneath says what of what is beneath n, depth levels beneath the top of
// the file and at the walker's path, is not sound to be read, in the order it
// is written: the first of it, when that is deeper than MaxDepth, and nothing
// else; and otherwise of each entry of a mapping, an anchor or an alias of
// its key or of its value, and then its key not being a string, under the
// entry's path, and of each item of a sequence its being an anchor or an
// alias, and what is beneath each of them.
func (w *walker) beneath(n *yaml.Node, depth int) {
	switch {
	case w.full() || len(n.Content) == 0:
		return
	case depth >= MaxDepth:
		first := step{item: true}
		if n.Kind == yaml.MappingNode {
			first = step{key: keyOf(n.Content[0])}
		}

		w.steps = append(w.steps, first)
		w.refuse(w.path(), CodeTooDeep)
		w.steps = w.steps[:len(w.steps)-1]

		return
	}

	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content) && !w.full(); i += 2 {
			key, value := n.Content[i], n.Content[i+1]

			w.steps = append(w.steps, step{key: keyOf(key)})

			if aliasing(key) || aliasing(value) {
				w.refuse(w.path(), CodeUnsupportedAlias)
			}

			if !stringKey(key) {
				w.refuse(w.path(), CodeUnsupportedKey)
			}

			w.beneath(key, depth+1)
			w.beneath(value, depth+1)

			w.steps = w.steps[:len(w.steps)-1]
		}

	case yaml.SequenceNode:
		for i, item := range n.Content {
			if w.full() {
				break
			}

			w.steps = append(w.steps, step{index: i, item: true})
			w.walk(item, depth+1)
			w.steps = w.steps[:len(w.steps)-1]
		}
	}
}

// path is where the walker is: its steps, written out as a path, as field
// and indexed write one.
func (w *walker) path() string {
	var b strings.Builder

	size := 0
	for _, s := range w.steps {
		// a key and the dot before it, or an index in brackets.
		size += len(s.key) + 8
	}

	b.Grow(size)

	for _, s := range w.steps {
		switch {
		case s.item:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(s.index))
			b.WriteByte(']')
		case b.Len() > 0:
			b.WriteByte('.')
			b.WriteString(s.key)
		default:
			b.WriteString(s.key)
		}
	}

	return b.String()
}

// shaped reports whether a stack's file is shaped as the platform reads one,
// saying of each mapping whose keys are fixed what is not, in the order the
// file says them: a key it does not take, or takes once and is given twice,
// under its own path, a value beneath one that is not shaped as it is to be,
// a name that is not one, and more services, volumes or networks than it
// takes.
func (r *reading) shaped(root *yaml.Node) bool {
	before := len(r.invalid)

	if entry(root, "services") == nil {
		r.refuse("services", "required_field")
	}

	r.keys(root, here(""), fileKeys, func(key string, path string, value *yaml.Node) {
		switch key {
		case "services":
			r.servicesShaped(value, path)
		case "volumes":
			r.declaredShaped(value, path, MaxVolumes, r.declared.volumes)
		case "networks":
			r.declaredShaped(value, path, MaxNetworks, r.declared.networks)
		}
	})

	return len(r.invalid) == before
}

// servicesShaped says what of the services of a stack's file, at path, is not
// shaped as they are to be: none, more than MaxServices, a name that is not
// one (each), a service that is not a mapping, and a key of one, of its
// resources or of its limits it does not take, or is given twice. Each is
// declared by its name as it is written.
func (r *reading) servicesShaped(services *yaml.Node, path string) {
	switch {
	case isNull(services) || (services.Kind == yaml.MappingNode && len(services.Content) == 0):
		r.refuse(path, "required_field")
	case services.Kind != yaml.MappingNode:
		r.refuse(path, "invalid_value")
	case len(services.Content)/2 > MaxServices:
		r.refuse(path, "too_large")
	default:
		r.each(services, path, func(name string, at place, service *yaml.Node) bool {
			if service.Kind != yaml.MappingNode {
				r.refuse(at.String(), "invalid_value")

				return true
			}

			r.declared.services = append(r.declared.services, name)
			r.declared.nodes[name] = service

			r.keys(service, at, serviceKeys, func(key string, path string, resources *yaml.Node) {
				if key != "resources" || !r.mappingOrNull(resources, here(path)) {
					return
				}

				r.keys(resources, here(path), resourcesKeys, func(_ string, path string, limits *yaml.Node) {
					if r.mappingOrNull(limits, here(path)) {
						r.keys(limits, here(path), limitsKeys, nil)
					}
				})
			})

			return true
		})
	}
}

// declaredShaped says what of the volumes or the networks a stack's file
// declares, at path, is not shaped as they are to be: more than most, a name
// that is not one (each), and one with anything beneath it. Each is declared
// among names by its name as it is written.
func (r *reading) declaredShaped(declared *yaml.Node, path string, most int, names map[string]bool) {
	switch {
	case !r.mappingOrNull(declared, here(path)):
	case len(declared.Content)/2 > most:
		r.refuse(path, "too_large")
	default:
		r.each(declared, path, func(name string, at place, value *yaml.Node) bool {
			names[name] = true

			if r.mappingOrNull(value, at) {
				r.keys(value, at, nil, nil)
			}

			return true
		})
	}
}

// keys says of a mapping, at, each key it does not take, and each it is
// given twice, under its own path, and has what of each other, under its
// path, in the order the mapping gives them, when there is a what. A mapping
// that is not there takes any. Where the mapping is is written out once a key
// of it is refused, or what of one is had, and not before; and takes holds
// 64 keys at most.
func (r *reading) keys(mapping *yaml.Node, at place, takes []string, what func(key string, path string, value *yaml.Node)) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}

	var (
		path    string
		written bool
		seen    uint64
	)

	under := func(key string) string {
		if !written {
			path, written = at.String(), true
		}

		return field(path, key)
	}

	for i := 0; i+1 < len(mapping.Content) && !r.full(); i += 2 {
		key := keyOf(mapping.Content[i])

		switch taken := slices.Index(takes, key); {
		case taken < 0:
			r.refuse(under(key), CodeUnsupportedKey)
		case seen&(1<<taken) != 0:
			r.refuse(under(key), CodeDuplicateKey)
		default:
			seen |= 1 << taken

			if what != nil {
				what(key, under(key), mapping.Content[i+1])
			}
		}
	}
}

// each has what of each entry of a mapping at path, by its name, in the
// order the mapping gives them, for as long as what says to go on: each of
// them has to be a name (nameCode) and be given once, under the entry's
// place, and one that is not is said once, and nothing beneath it is read.
func (r *reading) each(mapping *yaml.Node, path string, what func(name string, at place, value *yaml.Node) bool) {
	seen := make(map[string]bool)

	for i := 0; i+1 < len(mapping.Content) && !r.full(); i += 2 {
		name := keyOf(mapping.Content[i])
		at := entryOf(path, name)

		switch code := nameCode(name); {
		case len(code) > 0:
			r.refuse(at.String(), code)
		case seen[name]:
			r.refuse(at.String(), CodeDuplicateKey)
		default:
			seen[name] = true

			if !what(name, at, mapping.Content[i+1]) {
				return
			}
		}
	}
}

// mappingOrNull reports whether a value at is a mapping, saying it is not
// when it is anything else but nothing: what has nothing beneath it is
// shaped as anything is.
func (r *reading) mappingOrNull(value *yaml.Node, at place) bool {
	switch {
	case isNull(value):
		return false
	case value.Kind != yaml.MappingNode:
		r.refuse(at.String(), "invalid_value")

		return false
	}

	return true
}

// file is a stack's file, read off the nodes its walk read (shaped): its
// services in the order it says them, each of their values under its path,
// and held to what it names being there, and the volumes and the networks it
// declares, each by its name as it writes it.
func (r *reading) file() File {
	f := File{
		Services: make(map[string]Service, len(r.declared.services)),
		Volumes:  slices.Sorted(maps.Keys(r.declared.volumes)),
		Networks: slices.Sorted(maps.Keys(r.declared.networks)),
	}

	for _, name := range r.declared.services {
		if r.full() {
			break
		}

		f.Services[name] = r.service(field("services", name), r.declared.nodes[name])
	}

	return f
}

// service is one service of a stack's file, n as it is written, read under
// path: each of its values in the order it says them. One that names no
// image is refused for it first.
func (r *reading) service(path string, n *yaml.Node) Service {
	s := Service{Restart: RestartNo}

	if entry(n, "image") == nil {
		r.refuse(field(path, "image"), "required_field")
	}

	r.keys(n, here(path), serviceKeys, func(key string, at string, value *yaml.Node) {
		switch key {
		case "image":
			s.Image = r.image(value, at)
		case "command":
			s.Command = r.words(value, at)
		case "entrypoint":
			s.Entrypoint = r.words(value, at)
		case "environment":
			s.Environment = r.environment(value, at)
		case "ports":
			s.Ports = r.ports(value, at)
		case "privileged":
			s.Privileged = r.boolean(value, at)
		case "restart":
			s.Restart = r.restart(value, at)
		case "depends_on":
			s.DependsOn = r.dependsOn(value, at)
		case "volumes":
			s.Volumes = r.volumes(value, at)
		case "networks":
			s.Networks = r.networks(value, at)
		case "pids_limit":
			s.PidsLimit = r.pidsLimit(value, at)
		case "user":
			s.User = r.text(value, at)
		case "runtime":
			s.Runtime = r.text(value, at)
		case "resources":
			s.CPUs, s.Memory = r.limits(value, at)
		}
	})

	// one that names no network joins the default one, which is as much a
	// reference as one it names, under its networks.
	if len(s.Networks) == 0 && r.refer(entryOf(path, "networks")) {
		s.Networks = []string{DefaultNetwork}
	}

	return s
}

// refer counts one more reference the file's services make, at, and reports
// whether that is within MaxReferences, all of them together: the one that
// would take them past it is refused as too_large, and no reference after it
// is counted, or read.
func (r *reading) refer(at place) bool {
	switch {
	case r.referencesCapped:
		return false
	case r.references >= MaxReferences:
		r.refuse(at.String(), "too_large")
		r.referencesCapped = true

		return false
	}

	r.references++

	return true
}

// limits are the cores and the bytes a service's resources, n, at path, hold
// it to, each read in the order its limits say them: nothing when they say
// nothing.
func (r *reading) limits(n *yaml.Node, path string) (cpus float64, memory uint64) {
	r.keys(entry(n, "limits"), here(field(path, "limits")), limitsKeys, func(key string, at string, value *yaml.Node) {
		switch key {
		case "cpus":
			cpus = r.cpus(value, at)
		case "memory":
			memory = r.memory(value, at)
		}
	})

	return cpus, memory
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
	if n.Kind != yaml.SequenceNode {
		r.refuse(path, "invalid_value")

		return nil
	}

	values := make([]string, 0, len(n.Content))

	for i, item := range n.Content {
		if r.full() {
			break
		}

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
// each once, or a list of KEY=VALUE. A key is something, and has no = in it.
// The path of a variable is written out only once it is refused.
func (r *reading) environment(n *yaml.Node, path string) []string {
	if isNull(n) {
		return nil
	}

	var variables []string

	switch n.Kind {
	case yaml.MappingNode:
		seen := make(map[string]bool)

		for i := 0; i+1 < len(n.Content) && !r.full(); i += 2 {
			key := keyOf(n.Content[i])

			value, ok := scalar(n.Content[i+1])
			if isNull(n.Content[i+1]) {
				value, ok = "", true
			}

			switch {
			case seen[key]:
				r.refuse(field(path, key), CodeDuplicateKey)
			case !ok || len(key) == 0 || strings.Contains(key, "="):
				r.refuse(field(path, key), "invalid_value")
			default:
				variables = append(variables, key+"="+value)
			}

			seen[key] = true
		}

	case yaml.SequenceNode:
		for i, item := range n.Content {
			if r.full() {
				break
			}

			variable, ok := scalar(item)
			if key, _, cut := strings.Cut(variable, "="); !ok || isNull(item) || !cut || len(key) == 0 {
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
// A port that would take what the file publishes past MaxPorts is refused,
// counted by the size of its ranges before any port of them is, and no port
// is read after it. The path of a port is written out only once something of
// it is refused.
func (r *reading) ports(n *yaml.Node, path string) []Port {
	if isNull(n) {
		return nil
	}

	if n.Kind != yaml.SequenceNode {
		r.refuse(path, "invalid_value")

		return nil
	}

	var ports []Port

	for i, item := range n.Content {
		if r.full() || r.capped {
			break
		}

		if item.Kind == yaml.MappingNode {
			if p := r.longPort(item, itemOf(path, i)); r.publish(1, itemOf(path, i)) {
				ports = append(ports, p)
			}

			continue
		}

		written, ok := scalar(item)
		if !ok || isNull(item) {
			r.refuse(indexed(path, i), CodeInvalidPortMapping)

			continue
		}

		published, code := shortPort(written)

		switch {
		case len(code) > 0:
			r.refuse(indexed(path, i), code)
		case r.publish(published.containers.size(), itemOf(path, i)):
			ports = append(ports, published.ports()...)
		}
	}

	return ports
}

// publish counts as many ports more published, at, and reports whether that
// is within MaxPorts, every port the file's services publish together: what
// would take them past it is refused, and nothing is published after it.
func (r *reading) publish(ports int, at place) bool {
	if r.capped || r.published+ports > MaxPorts {
		r.refuse(at.String(), "too_large")
		r.capped = true

		return false
	}

	r.published += ports

	return true
}

// longPort is a port a service publishes, at, in its long form: target,
// published, host_ip and protocol, the target one port and published one or
// none.
func (r *reading) longPort(n *yaml.Node, at place) Port {
	r.keys(n, at, longPortKeys, nil)

	p := Port{Protocol: ProtocolTCP}

	target, ok := portOf(entry(n, "target"))
	if !ok {
		r.refuse(field(at.String(), "target"), CodeInvalidPortMapping)
	}

	p.Container = target

	if published := entry(n, "published"); !isNull(published) {
		if p.Host, ok = portOf(published); !ok {
			r.refuse(field(at.String(), "published"), CodeInvalidPortMapping)
		}
	}

	if hostIP := entry(n, "host_ip"); !isNull(hostIP) {
		if p.HostIP, ok = scalar(hostIP); !ok || !isIP(p.HostIP) {
			r.refuse(field(at.String(), "host_ip"), "invalid_ip")
		}
	}

	if protocol := entry(n, "protocol"); !isNull(protocol) {
		if p.Protocol, ok = scalar(protocol); !ok || (p.Protocol != ProtocolTCP && p.Protocol != ProtocolUDP) {
			r.refuse(field(at.String(), "protocol"), "invalid_protocol")
		}
	}

	return p
}

// portMapping is a port a service publishes in its short form, as it is
// written, its ranges by their ends, before any port of them is counted out
// (ports).
type portMapping struct {
	// containers are its container ports, and hosts as many on the VM, or
	// none, each of the containers' then published on a port docker picks.
	containers portSpan
	hosts      portSpan

	hostIP   string
	protocol string
}

// ports are the ports m publishes, each port of its ranges one.
func (m portMapping) ports() []Port {
	ports := make([]Port, m.containers.size())
	for i := range ports {
		ports[i] = Port{Container: m.containers.first + port.Port(i), HostIP: m.hostIP, Protocol: m.protocol}

		if m.hosts.size() > 0 {
			ports[i].Host = m.hosts.first + port.Port(i)
		}
	}

	return ports
}

// portSpan is the ports from first to last, both among them: none when it is
// nothing.
type portSpan struct {
	first port.Port
	last  port.Port
}

// size is how many ports s is.
func (s portSpan) size() int {
	if s.first == 0 {
		return 0
	}

	return int(s.last-s.first) + 1
}

// shortPort is what [ip:][host:]container[/tcp|udp] publishes, ranges of as
// many ports on either side being each of them, or the code it is refused
// with: an address in brackets when it is an IPv6 one, a host that says no
// port being one docker picks, and a range of container ports with none on
// the host each with one docker picks.
func shortPort(written string) (portMapping, string) {
	mapping, protocol, protocolled := strings.Cut(written, "/")
	if !protocolled {
		protocol = ProtocolTCP
	}

	if protocol != ProtocolTCP && protocol != ProtocolUDP {
		return portMapping{}, "invalid_protocol"
	}

	var hostIP string

	if strings.HasPrefix(mapping, "[") {
		closing := strings.Index(mapping, "]:")
		if closing < 0 {
			return portMapping{}, CodeInvalidPortMapping
		}

		hostIP, mapping = mapping[1:closing], mapping[closing+2:]

		switch {
		case !isIP(hostIP):
			return portMapping{}, "invalid_ip"
		case !strings.Contains(mapping, ":"):
			return portMapping{}, CodeInvalidPortMapping
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
			return portMapping{}, CodeInvalidPortMapping
		}

		hostIP, host, container = parts[0], parts[1], parts[2]

		if !isIP(hostIP) {
			return portMapping{}, "invalid_ip"
		}
	default:
		return portMapping{}, CodeInvalidPortMapping
	}

	containers, ok := portRange(container)
	if !ok {
		return portMapping{}, CodeInvalidPortMapping
	}

	var hosts portSpan

	if len(host) > 0 {
		if hosts, ok = portRange(host); !ok || hosts.size() != containers.size() {
			return portMapping{}, CodeInvalidPortMapping
		}
	}

	return portMapping{containers: containers, hosts: hosts, hostIP: hostIP, protocol: protocol}, ""
}

// portRange is the ports a port, or a range of them first-last, says, and
// whether it says any: a port is from 1 to 65535.
func portRange(written string) (portSpan, bool) {
	first, last, ranged := strings.Cut(written, "-")
	if !ranged {
		last = first
	}

	from, fromOK := parsePort(first)
	to, toOK := parsePort(last)

	if !fromOK || !toOK || to < from {
		return portSpan{}, false
	}

	return portSpan{first: from, last: to}, true
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
// condition it is waited on with, or to nothing, which is that one. Each is
// a reference (refer), held to being one of the file's services as it is
// read, and so to its name being of MaxNameLength characters at most; and
// the path of each is written out only once something of it is refused.
func (r *reading) dependsOn(n *yaml.Node, path string) []string {
	if isNull(n) {
		return nil
	}

	var services []string

	switch n.Kind {
	case yaml.SequenceNode:
		for i, item := range n.Content {
			if r.full() || r.referencesCapped {
				break
			}

			service, ok := scalar(item)

			switch {
			case !ok || isNull(item):
				r.refuse(indexed(path, i), "invalid_value")
			case len(service) > MaxNameLength:
				r.refuse(indexed(path, i), "too_large")
			case slices.Contains(services, service):
				r.refuse(indexed(path, i), CodeDuplicateKey)
			case r.refer(itemOf(path, i)):
				services = append(services, service)
				r.defined(service, entryOf(path, service))
			}
		}

	case yaml.MappingNode:
		seen := make(map[string]bool)

		for i := 0; i+1 < len(n.Content) && !r.full() && !r.referencesCapped; i += 2 {
			service := keyOf(n.Content[i])
			at := entryOf(path, service)

			switch {
			case len(service) > MaxNameLength:
				r.refuse(at.String(), "too_large")

				continue
			case seen[service]:
				r.refuse(at.String(), CodeDuplicateKey)

				continue
			}

			seen[service] = true

			if condition := n.Content[i+1]; !isNull(condition) {
				if !r.mappingOrNull(condition, at) {
					continue
				}

				r.keys(condition, at, conditionKeys, nil)

				if waited, ok := scalar(entry(condition, "condition")); !isNull(entry(condition, "condition")) && (!ok || waited != ConditionStarted) {
					r.refuse(field(at.String(), "condition"), CodeInvalidCondition)
				}
			}

			if !r.refer(at) {
				break
			}

			services = append(services, service)
			r.defined(service, at)
		}

	default:
		r.refuse(path, "invalid_value")
	}

	slices.Sort(services)

	return services
}

// defined says at that the file has no service called service, when it has
// none.
func (r *reading) defined(service string, at place) {
	if _, defined := r.declared.nodes[service]; !defined {
		r.refuse(at.String(), CodeUndefinedService)
	}
}

// volumes are what a service mounts: source:target[:ro|rw], or type, source,
// target and read_only, a bind of an absolute path of its VM, or a volume
// by its name, which its file declares, as it is held to as it is read, and
// which is a reference (refer): no mount is read after the reference that
// would take what the file makes past the most it may. The path of a mount
// is written out only once it is refused.
func (r *reading) volumes(n *yaml.Node, path string) []Mount {
	if isNull(n) {
		return nil
	}

	if n.Kind != yaml.SequenceNode {
		r.refuse(path, "invalid_value")

		return nil
	}

	var mounts []Mount

	for i, item := range n.Content {
		if r.full() || r.referencesCapped {
			break
		}

		var (
			m    Mount
			read bool
		)

		if item.Kind == yaml.MappingNode {
			m, read = r.longVolume(item, itemOf(path, i))
		} else {
			written, ok := scalar(item)
			if !ok || isNull(item) {
				r.refuse(indexed(path, i), "invalid_mount")

				continue
			}

			var code string
			if m, code = shortVolume(written); len(code) > 0 {
				r.refuse(indexed(path, i), code)

				continue
			}

			read = true
		}

		if !read {
			continue
		}

		if m.Type == MountVolume {
			if !r.refer(itemOf(path, i)) {
				break
			}

			if !r.declared.volumes[m.Source] {
				r.refuse(indexed(path, i), CodeUndefinedVolume)
			}
		}

		mounts = append(mounts, m)
	}

	return mounts
}

// longVolume is what a service mounts, at, in its long form, and whether it
// can be read as a mount, saying at what is wrong with it otherwise.
func (r *reading) longVolume(n *yaml.Node, at place) (Mount, bool) {
	before := len(r.invalid)

	r.keys(n, at, longVolumeKeys, nil)

	if len(r.invalid) > before {
		return Mount{}, false
	}

	refused := func(code string) (Mount, bool) {
		r.refuse(at.String(), code)

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
// that is not absolute, a volume with no name or a name that is none
// (nameCode), and a target that is not an absolute path.
func mountCode(m Mount) string {
	switch {
	case len(m.Source) == 0 && m.Type == MountVolume:
		return CodeAnonymousVolume
	case m.Type == MountBind && !strings.HasPrefix(m.Source, "/"):
		return CodeRelativeBind
	case m.Type == MountVolume && len(nameCode(m.Source)) > 0:
		return nameCode(m.Source)
	case !strings.HasPrefix(m.Target, "/"):
		return "invalid_mount"
	}

	return ""
}

// networks are the networks a service names, in the order of their names,
// each once: a list, or a mapping of each to nothing; none when it names none,
// when it joins the default one (service). Each is a reference (refer), held
// to being one the file declares as it is read, but for the default one, and
// to being a name (nameCode); and the path of each is written out only once
// something of it is refused.
func (r *reading) networks(n *yaml.Node, path string) []string {
	if isNull(n) {
		return nil
	}

	var networks []string

	switch n.Kind {
	case yaml.SequenceNode:
		for i, item := range n.Content {
			if r.full() || r.referencesCapped {
				break
			}

			network, ok := scalar(item)
			if !ok || isNull(item) {
				r.refuse(indexed(path, i), "invalid_value")

				continue
			}

			switch code := nameCode(network); {
			case len(code) > 0:
				r.refuse(indexed(path, i), code)
			case slices.Contains(networks, network):
				r.refuse(indexed(path, i), CodeDuplicateKey)
			case r.refer(itemOf(path, i)):
				networks = append(networks, network)
				r.joins(network, entryOf(path, network))
			}
		}

	case yaml.MappingNode:
		r.each(n, path, func(network string, at place, value *yaml.Node) bool {
			if !r.refer(at) {
				return false
			}

			if r.mappingOrNull(value, at) {
				r.keys(value, at, nil, nil)
			}

			networks = append(networks, network)
			r.joins(network, at)

			return true
		})

	default:
		r.refuse(path, "invalid_value")
	}

	slices.Sort(networks)

	return networks
}

// joins says at that the file declares no network called network, when it
// declares none, but for the default one, which needs no declaring.
func (r *reading) joins(network string, at place) {
	if !r.declared.networks[network] && network != DefaultNetwork {
		r.refuse(at.String(), CodeUndefinedNetwork)
	}
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
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if keyOf(mapping.Content[i]) == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

// keyOf is a mapping's key as it is written: an alias as the name it is
// written with, which the file is refused for (CodeUnsupportedAlias).
func keyOf(key *yaml.Node) string {
	return key.Value
}

// isNull reports whether n says nothing: there is none, or it is null.
func isNull(n *yaml.Node) bool {
	return n == nil || n.Kind == 0 || (n.Kind == yaml.ScalarNode && n.Tag == "!!null")
}

// scalar is what a scalar says, as it is written, and whether n is one.
func scalar(n *yaml.Node) (string, bool) {
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

// place is where something is in a stack's file: the path of what holds it,
// and its key there, or its index, when what holds it is a sequence. It is
// written out as a path (String) only once something there is refused, or
// read beneath it, so that what of a file is read and not refused costs
// nothing of where it is, however many items it has.
type place struct {
	within string
	key    string
	index  int
	item   bool
}

// here is the place path is.
func here(path string) place {
	return place{key: path}
}

// entryOf is the place of the entry of the mapping at path whose key is key.
func entryOf(path string, key string) place {
	return place{within: path, key: key}
}

// itemOf is the place of the item at i of the sequence at path.
func itemOf(path string, i int) place {
	return place{within: path, index: i, item: true}
}

// String is the path of p, as field and indexed write one.
func (p place) String() string {
	if p.item {
		return indexed(p.within, p.index)
	}

	return field(p.within, p.key)
}
