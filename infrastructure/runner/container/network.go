package container

import (
	"context"
	"slices"
	"strings"

	containerTypes "github.com/docker/docker/api/types/container"
	networkTypes "github.com/docker/docker/api/types/network"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
)

// networkMode is the network a container is created on. Docker takes exactly
// one at create time; anything else is connected afterwards.
func networkMode(attachments []network.Attachment) containerTypes.NetworkMode {
	if len(attachments) == 0 {
		return containerTypes.NetworkMode(network.NoNetworkName)
	}

	return containerTypes.NetworkMode(attachments[0].Name)
}

// endpointsConfig carries the names a container answers to on the network it is
// created on, so its neighbours reach it by service name.
func endpointsConfig(attachments []network.Attachment) *networkTypes.NetworkingConfig {
	if len(attachments) == 0 {
		return nil
	}

	settings := endpointSettings(attachments[0])
	if settings == nil {
		return nil
	}

	return &networkTypes.NetworkingConfig{
		EndpointsConfig: map[string]*networkTypes.EndpointSettings{
			attachments[0].Name: settings,
		},
	}
}

// gatewayPriority puts the network that routes out ahead of the ones that do
// not, so a container on several of them has its default route through the one
// that can actually carry traffic off the node.
const gatewayPriority = 100

// endpointSettings is what a container is joined to one network with, or nil
// when there is nothing to say about it.
func endpointSettings(attachment network.Attachment) *networkTypes.EndpointSettings {
	if len(attachment.Aliases) == 0 && !attachment.Gateway {
		return nil
	}

	settings := &networkTypes.EndpointSettings{Aliases: attachment.Aliases}
	if attachment.Gateway {
		settings.GwPriority = gatewayPriority
	}

	return settings
}

// connectRemainingNetworks joins the container to everything beyond the network
// it was created on.
func (m *DockerManager) connectRemainingNetworks(ctx context.Context, containerID string, attachments []network.Attachment) error {
	for _, attachment := range attachments[min(1, len(attachments)):] {
		settings := endpointSettings(attachment)
		if settings == nil {
			settings = &networkTypes.EndpointSettings{}
		}

		if err := m.client.NetworkConnect(ctx, attachment.Name, containerID, settings); err != nil {
			return err
		}
	}

	return nil
}

// inspectedNetworks reads back the networks a container is on, so that what a
// container reports about itself is the same shape as what it was asked for.
func inspectedNetworks(settings *containerTypes.NetworkSettings) []network.Attachment {
	if settings == nil {
		return nil
	}

	attachments := make([]network.Attachment, 0, len(settings.Networks))
	for name, endpoint := range settings.Networks {
		attachment := network.Attachment{Name: name}
		if endpoint != nil {
			attachment.Aliases = endpoint.Aliases
		}

		attachments = append(attachments, attachment)
	}

	slices.SortFunc(attachments, func(a network.Attachment, b network.Attachment) int {
		return strings.Compare(a.Name, b.Name)
	})

	return attachments
}
