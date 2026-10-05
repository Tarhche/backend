package docker

import (
	"bytes"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

const (
	streamStdout = "stdout"
	streamStderr = "stderr"

	// defaultProtocol is the protocol of a port that names none.
	defaultProtocol = "tcp"

	// logLineLimit caps a single line, so a container writing one enormous
	// line without a newline cannot grow what holds it without bound.
	logLineLimit = 64 << 10
)

// containerName is a container's name as docker lists it, without the slash
// docker puts in front of it.
func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}

	return strings.TrimPrefix(names[0], "/")
}

func fromSummary(summary container.Summary) docker.Container {
	c := docker.Container{
		ID:        summary.ID,
		Name:      containerName(summary.Names),
		Image:     summary.Image,
		State:     summary.State,
		Status:    summary.Status,
		Command:   summary.Command,
		Labels:    maps.Clone(summary.Labels),
		Stack:     summary.Labels[docker.LabelComposeProject],
		Service:   summary.Labels[docker.LabelComposeService],
		CreatedAt: time.Unix(summary.Created, 0).UTC(),
	}

	// a port published on both IPv4 and IPv6 is listed twice, and is one
	// binding.
	for _, p := range summary.Ports {
		if p.PublicPort == 0 {
			continue
		}

		binding := docker.PortBinding{
			ContainerPort: port.Port(p.PrivatePort),
			HostPort:      port.Port(p.PublicPort),
			Protocol:      p.Type,
		}

		if !slices.Contains(c.Ports, binding) {
			c.Ports = append(c.Ports, binding)
		}
	}

	if summary.NetworkSettings != nil {
		c.Networks = slices.Sorted(maps.Keys(summary.NetworkSettings.Networks))
	}

	for _, m := range summary.Mounts {
		c.Mounts = append(c.Mounts, fromMountPoint(m))
	}

	return c
}

func fromInspect(inspected container.InspectResponse) docker.Container {
	var c docker.Container

	if base := inspected.ContainerJSONBase; base != nil {
		c.ID = base.ID
		c.Name = strings.TrimPrefix(base.Name, "/")
		c.Command = strings.TrimSpace(strings.Join(append([]string{base.Path}, base.Args...), " "))

		if created, err := time.Parse(time.RFC3339Nano, base.Created); err == nil {
			c.CreatedAt = created.UTC()
		}

		if base.State != nil {
			c.State = string(base.State.Status)
			c.Status = statusOf(base.State)
		}

		if base.HostConfig != nil {
			c.RestartPolicy = string(base.HostConfig.RestartPolicy.Name)
		}
	}

	if inspected.Config != nil {
		c.Image = inspected.Config.Image
		c.Labels = maps.Clone(inspected.Config.Labels)
		c.Stack = inspected.Config.Labels[docker.LabelComposeProject]
		c.Service = inspected.Config.Labels[docker.LabelComposeService]
	}

	if settings := inspected.NetworkSettings; settings != nil {
		c.Networks = slices.Sorted(maps.Keys(settings.Networks))

		for _, guest := range slices.SortedFunc(maps.Keys(settings.Ports), func(a nat.Port, b nat.Port) int {
			return a.Int() - b.Int()
		}) {
			for _, published := range settings.Ports[guest] {
				hostPort, err := strconv.ParseUint(published.HostPort, 10, 16)
				if err != nil || hostPort == 0 {
					continue
				}

				binding := docker.PortBinding{
					ContainerPort: port.Port(guest.Int()),
					HostPort:      port.Port(hostPort),
					Protocol:      guest.Proto(),
				}

				if !slices.Contains(c.Ports, binding) {
					c.Ports = append(c.Ports, binding)
				}
			}
		}
	}

	for _, m := range inspected.Mounts {
		c.Mounts = append(c.Mounts, fromMountPoint(m))
	}

	return c
}

// statusOf is the sentence docker would put a container's state in.
func statusOf(state *container.State) string {
	switch state.Status {
	case container.StateRunning:
		return "Up"
	case container.StateExited:
		return "Exited (" + strconv.Itoa(state.ExitCode) + ")"
	default:
		return string(state.Status)
	}
}

func fromMountPoint(m container.MountPoint) docker.Mount {
	source := m.Source
	if m.Type == mount.TypeVolume && len(m.Name) > 0 {
		source = m.Name
	}

	return docker.Mount{
		Type:     string(m.Type),
		Source:   source,
		Target:   m.Destination,
		ReadOnly: !m.RW,
	}
}

// toCreate is a container spec as docker takes it at create time. Docker
// takes one network then, so a container on several is connected to the rest
// afterwards.
func toCreate(spec docker.ContainerSpec) (*container.Config, *container.HostConfig, *network.NetworkingConfig) {
	config := &container.Config{
		Image:        spec.Image,
		Cmd:          spec.Command,
		Entrypoint:   spec.Entrypoint,
		Env:          spec.Env,
		WorkingDir:   spec.WorkingDir,
		Labels:       spec.Labels,
		ExposedPorts: make(nat.PortSet, len(spec.Ports)),
	}

	// memory is in bytes here as it is everywhere else, and a CPU is counted
	// in billionths of one.
	hostConfig := &container.HostConfig{
		PortBindings:  make(nat.PortMap, len(spec.Ports)),
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(spec.RestartPolicy)},
		Resources: container.Resources{
			NanoCPUs: int64(spec.CPUs * 1e9),
			Memory:   int64(min(spec.Memory, math.MaxInt64)),
		},
	}

	for _, binding := range spec.Ports {
		protocol := binding.Protocol
		if len(protocol) == 0 {
			protocol = defaultProtocol
		}

		guest := nat.Port(strconv.FormatUint(uint64(binding.ContainerPort), 10) + "/" + protocol)
		config.ExposedPorts[guest] = struct{}{}

		// no host port is docker's own "pick a free one".
		hostPort := ""
		if binding.HostPort > 0 {
			hostPort = strconv.FormatUint(uint64(binding.HostPort), 10)
		}

		hostConfig.PortBindings[guest] = append(hostConfig.PortBindings[guest], nat.PortBinding{HostPort: hostPort})
	}

	for _, m := range spec.Mounts {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.Type(m.Type),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	if len(spec.Networks) == 0 {
		return config, hostConfig, nil
	}

	hostConfig.NetworkMode = container.NetworkMode(spec.Networks[0])

	return config, hostConfig, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{spec.Networks[0]: {}},
	}
}

// fromStats is one sample of what a container uses.
//
// The CPU percent is docker's own — the container's share of the CPU time
// that passed between the two samples, times the CPUs it sees — divided by the
// CPUs it sees: 100 is all of them busy, rather than 100 for each. Memory in
// use leaves out the page cache the kernel can take back, as `docker stats`
// does.
func fromStats(sample container.StatsResponse) docker.Stats {
	stats := docker.Stats{
		MemoryLimit: sample.MemoryStats.Limit,
		PIDs:        sample.PidsStats.Current,
		SampledAt:   sample.Read,
	}

	cpuDelta := float64(sample.CPUStats.CPUUsage.TotalUsage) - float64(sample.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(sample.CPUStats.SystemUsage) - float64(sample.PreCPUStats.SystemUsage)

	if cpuDelta > 0 && systemDelta > 0 {
		stats.CPUPercent = min(cpuDelta/systemDelta*100, 100)
	}

	stats.MemoryUsed = sample.MemoryStats.Usage
	for _, cache := range []string{"inactive_file", "total_inactive_file"} {
		if inactive, ok := sample.MemoryStats.Stats[cache]; ok && inactive < stats.MemoryUsed {
			stats.MemoryUsed -= inactive

			break
		}
	}

	for _, n := range sample.Networks {
		stats.NetworkRx += n.RxBytes
		stats.NetworkTx += n.TxBytes
	}

	for _, entry := range sample.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(entry.Op) {
		case "read":
			stats.BlockRead += entry.Value
		case "write":
			stats.BlockWrite += entry.Value
		}
	}

	return stats
}

func fromImageSummary(summary image.Summary, inUse bool) docker.Image {
	return docker.Image{
		ID:        summary.ID,
		Tags:      tagsOf(summary.RepoTags),
		Size:      summary.Size,
		CreatedAt: time.Unix(summary.Created, 0).UTC(),
		InUse:     inUse,
	}
}

func fromImageInspect(inspected image.InspectResponse) docker.Image {
	pulled := docker.Image{
		ID:   inspected.ID,
		Tags: tagsOf(inspected.RepoTags),
		Size: inspected.Size,
	}

	if created, err := time.Parse(time.RFC3339Nano, inspected.Created); err == nil {
		pulled.CreatedAt = created.UTC()
	}

	return pulled
}

// tagsOf is an image's tags, without the placeholder docker gives one that has
// none.
func tagsOf(repoTags []string) []string {
	tags := make([]string, 0, len(repoTags))
	for _, tag := range repoTags {
		if tag != "<none>:<none>" {
			tags = append(tags, tag)
		}
	}

	return tags
}

func fromNetwork(n network.Inspect, containers []string) docker.Network {
	return docker.Network{
		ID:         n.ID,
		Name:       n.Name,
		Driver:     n.Driver,
		Scope:      n.Scope,
		Internal:   n.Internal,
		Containers: containers,
		Labels:     maps.Clone(n.Labels),
		CreatedAt:  n.Created.UTC(),
	}
}

func fromVolume(v volume.Volume, inUse bool) docker.Volume {
	created := docker.Volume{
		Name:       v.Name,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		Labels:     maps.Clone(v.Labels),
		InUse:      inUse,
	}

	if at, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
		created.CreatedAt = at.UTC()
	}

	return created
}

// logLines gathers the lines of a container's log as they are demultiplexed,
// in the order they were written whichever stream they were on.
type logLines struct {
	lines   []docker.LogLine
	streams []*logStream
}

// stream is what one of the container's streams is written to.
func (l *logLines) stream(name string) *logStream {
	s := &logStream{lines: l, name: name}
	l.streams = append(l.streams, s)

	return s
}

// flush keeps whatever a stream wrote without a closing newline, which is
// still output.
func (l *logLines) flush() {
	for _, s := range l.streams {
		s.flush()
	}
}

type logStream struct {
	lines   *logLines
	name    string
	partial bytes.Buffer
}

func (s *logStream) Write(p []byte) (int, error) {
	s.partial.Write(p)

	for {
		line, err := s.partial.ReadBytes('\n')
		if err != nil {
			// an incomplete line goes back to be finished by what comes next,
			// unless it has grown past what one line may be.
			s.partial.Reset()
			s.partial.Write(line)

			if s.partial.Len() >= logLineLimit {
				s.flush()
			}

			return len(p), nil
		}

		s.keep(line)
	}
}

func (s *logStream) flush() {
	if s.partial.Len() == 0 {
		return
	}

	line := bytes.Clone(s.partial.Bytes())
	s.partial.Reset()
	s.keep(line)
}

// keep takes apart the "<rfc3339nano> <line>" docker writes when it is asked
// to timestamp a log. A line that somehow carries no timestamp is still worth
// keeping, at no particular time.
func (s *logStream) keep(raw []byte) {
	line := strings.TrimRight(string(raw), "\r\n")

	kept := docker.LogLine{Stream: s.name, Line: line}

	if stamp, content, found := strings.Cut(line, " "); found {
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			kept.At = at.UTC()
			kept.Line = content
		}
	}

	s.lines.lines = append(s.lines.lines, kept)
}
