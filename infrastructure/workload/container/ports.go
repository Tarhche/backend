package container

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

var _ task.Dialer = &DockerManager{}

// DialContext connects to one of a container's exposed ports through the port
// docker published it on, at the host the daemon publishes on. That port is
// read back off docker every time, because a container that restarted came
// back on a different one.
//
// It is what a node did before there was a Dialer, moved behind one: the
// orchestrator reaches every class's ports the same way, and a container's are
// reached where docker put them.
func (m *DockerManager) DialContext(ctx context.Context, containerUUID string, p port.Port) (net.Conn, error) {
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

// listedEndpoints reads off a listing which exposed ports docker published,
// which is what makes them reachable. Docker lists a published port once per
// address family it is bound on, so each is taken once.
func listedEndpoints(ports []containerTypes.Port) []port.Port {
	result := make([]port.Port, 0, len(ports))
	for _, p := range ports {
		if p.PublicPort == 0 || slices.Contains(result, port.Port(p.PrivatePort)) {
			continue
		}

		result = append(result, port.Port(p.PrivatePort))
	}

	slices.Sort(result)

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

	// docker hands the bindings back in no particular order, and whoever reads
	// them is told the lowest first.
	slices.Sort(result)

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

func convertPortSet(ports port.PortSet) nat.PortSet {
	result := make(nat.PortSet)
	for p := range ports {
		result[natPort(p)] = struct{}{}
	}
	return result
}

// convertPortMap turns the bindings a container asks for into docker's own
// shape. A binding with no host port asks docker to pick a free one, which is
// how the workload publishes a container's ports without having to keep track of
// what is already taken on the node.
func convertPortMap(bindings port.PortMap) nat.PortMap {
	result := make(nat.PortMap)
	for p, bindings := range bindings {
		portStr := natPort(p)
		result[portStr] = make([]nat.PortBinding, len(bindings))
		for i, b := range bindings {
			hostPort := ""
			if b.HostPort > 0 {
				hostPort = fmt.Sprintf("%d", b.HostPort)
			}

			result[portStr][i] = nat.PortBinding{
				HostIP:   b.HostIP,
				HostPort: hostPort,
			}
		}
	}
	return result
}

func convertDockerPortSet(ports []containerTypes.Port) port.PortSet {
	result := make(port.PortSet)
	for _, p := range ports {
		result[port.Port(p.PrivatePort)] = struct{}{}
	}
	return result
}

func convertDockerPortMap(ports []containerTypes.Port) port.PortMap {
	result := make(port.PortMap)
	for _, p := range ports {
		if p.PublicPort != 0 {
			result[port.Port(p.PrivatePort)] = []port.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: port.Port(p.PublicPort),
				},
			}
		}
	}
	return result
}

func convertDockerPortSetFromMap(ports nat.PortMap) port.PortSet {
	result := make(port.PortSet)
	for p := range ports {
		var portNum port.Port
		fmt.Sscanf(string(p), "%d/tcp", &portNum)
		result[portNum] = struct{}{}
	}
	return result
}

func convertDockerPortMapFromMap(ports nat.PortMap) port.PortMap {
	result := make(port.PortMap)
	for p, bindings := range ports {
		var portNum port.Port
		fmt.Sscanf(string(p), "%d/tcp", &portNum)
		result[portNum] = make([]port.PortBinding, len(bindings))
		for i, b := range bindings {
			var hostPort port.Port
			fmt.Sscanf(b.HostPort, "%d", &hostPort)
			result[portNum][i] = port.PortBinding{
				HostIP:   b.HostIP,
				HostPort: hostPort,
			}
		}
	}
	return result
}
