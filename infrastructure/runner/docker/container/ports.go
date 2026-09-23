package container

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Dial connects to one of a container's exposed ports through the port docker
// published it on. That port is read back off docker every time, because a
// container that restarted came back on a different one.
func (m *DockerManager) Dial(ctx context.Context, containerUUID string, p port.Port) (net.Conn, error) {
	ctx, span := m.tracer.Start(ctx, "docker.task.dial",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID), attribute.Int("task.port", int(p))),
	)
	defer span.End()

	info, err := m.client.ContainerInspect(ctx, containerUUID)
	if err != nil {
		return nil, trace.RecordError(span, gone(err))
	}

	if info.NetworkSettings == nil {
		return nil, trace.RecordError(span, fmt.Errorf("container %s is on no network", containerUUID))
	}

	hostPort, found := publishedPort(info.NetworkSettings.Ports, p)
	if !found {
		return nil, trace.RecordError(span, fmt.Errorf("port %d of container %s is not published", p, containerUUID))
	}

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.advertiseHost, hostPort))
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	return conn, nil
}

// exposedPorts is the ports a container declares open, in docker's own shape.
func exposedPorts(ports port.PortSet) nat.PortSet {
	result := make(nat.PortSet, len(ports))
	for p := range ports {
		result[natPort(p)] = struct{}{}
	}

	return result
}

// publishAll publishes every exposed port on a host port docker picks, so the
// runner never has to keep track of what is already taken on the node.
func publishAll(ports port.PortSet) nat.PortMap {
	result := make(nat.PortMap, len(ports))
	for p := range ports {
		result[natPort(p)] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
	}

	return result
}

// listedExposedPorts reads the ports a container exposes off a listing.
func listedExposedPorts(ports []types.Port) port.PortSet {
	result := make(port.PortSet, len(ports))
	for _, p := range ports {
		result[port.Port(p.PrivatePort)] = struct{}{}
	}

	return result
}

// listedEndpoints reads off a listing which exposed ports docker published,
// which is what makes them reachable.
func listedEndpoints(ports []types.Port) []port.Port {
	result := make([]port.Port, 0, len(ports))
	for _, p := range ports {
		if p.PublicPort == 0 || slices.Contains(result, port.Port(p.PrivatePort)) {
			continue
		}

		result = append(result, port.Port(p.PrivatePort))
	}

	return result
}

// inspectedExposedPorts reads the ports a container exposes off an
// inspection.
func inspectedExposedPorts(ports nat.PortMap) port.PortSet {
	result := make(port.PortSet, len(ports))
	for p := range ports {
		result[port.Port(p.Int())] = struct{}{}
	}

	return result
}

// inspectedEndpoints reads off an inspection which exposed ports docker
// published.
func inspectedEndpoints(ports nat.PortMap) []port.Port {
	result := make([]port.Port, 0, len(ports))
	for p := range ports {
		if _, published := publishedPort(ports, port.Port(p.Int())); published {
			result = append(result, port.Port(p.Int()))
		}
	}

	return result
}

// publishedPort is the host port docker published one of a container's ports
// on, if it did.
func publishedPort(ports nat.PortMap, p port.Port) (string, bool) {
	for _, binding := range ports[natPort(p)] {
		if hostPort, err := strconv.Atoi(binding.HostPort); err == nil && hostPort > 0 {
			return binding.HostPort, true
		}
	}

	return "", false
}

func natPort(p port.Port) nat.Port {
	return nat.Port(fmt.Sprintf("%d/tcp", p))
}
