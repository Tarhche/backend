package client

import (
	"context"
	"net/url"

	"go.opentelemetry.io/otel/attribute"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// NodeManager says what a node's microVMs are using between them, which is
// what its heartbeat reports and the control plane schedules on.
type NodeManager struct {
	client *Client
}

var _ node.Manager = &NodeManager{}

func NewNodeManager(client *Client) *NodeManager {
	return &NodeManager{client: client}
}

// Stats is what the named node's running runs use between them. The service
// sums them, so a heartbeat costs one request however many runs there are,
// where docker is asked about each container in turn. A node that is not named
// holds nothing, and uses nothing.
func (m *NodeManager) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	ctx, span := m.client.span(ctx, "node.stats", attribute.String("node.name", nodeName))
	defer span.End()

	if len(nodeName) == 0 {
		return node.Stats{}, nil
	}

	var stats api.Stats
	if err := m.client.call(ctx, request{
		route: api.RouteNodeStats,
		query: url.Values{api.QueryNode: {nodeName}},
	}, &stats); err != nil {
		return node.Stats{}, trace.RecordError(span, err)
	}

	return nodeStats(stats), nil
}
