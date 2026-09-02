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
	if len(attachments) == 0 || len(attachments[0].Aliases) == 0 {
		return nil
	}

	return &networkTypes.NetworkingConfig{
		EndpointsConfig: map[string]*networkTypes.EndpointSettings{
			attachments[0].Name: {Aliases: attachments[0].Aliases},
		},
	}
}

// connectRemainingNetworks joins the container to everything beyond the network
// it was created on.
func (m *DockerManager) connectRemainingNetworks(ctx context.Context, containerID string, attachments []network.Attachment) error {
	for _, attachment := range attachments[min(1, len(attachments)):] {
		settings := &networkTypes.EndpointSettings{}
		if len(attachment.Aliases) > 0 {
			settings.Aliases = attachment.Aliases
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
