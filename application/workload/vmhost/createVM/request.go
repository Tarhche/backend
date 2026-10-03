package createVM

import (
	"math"
	"path"
	"regexp"
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// maxNetworks is how many networks one VM may join. A task joins at most
	// two: its own and the public one. Four leaves room without leaving it
	// open.
	maxNetworks = 4

	// maxExposedPorts is how many ports one VM may be reached on.
	maxExposedPorts = 64
)

var (
	// namePattern is what a VM may be called, as a container may be.
	namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)

	// hostnamePattern is what a VM may call itself: one label of a hostname.
	hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

	// aliasPattern is what a VM's neighbours may reach it by: a stack's
	// service names, as compose writes them.
	aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
)

// Request is a VM to make. It comes from an orchestrator, which is trusted to
// ask for what its tasks need, and is checked all the same: vmhost is root on
// the host, and what it is asked ends up in paths, devices and a guest's
// /etc/hosts.
type Request struct {
	vm.Spec
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.Name) == 0:
		validationErrors["name"] = "required_field"
	case !namePattern.MatchString(r.Name):
		validationErrors["name"] = "invalid_value"
	}

	if len(r.Hostname) > 0 && !hostnamePattern.MatchString(r.Hostname) {
		validationErrors["hostname"] = "invalid_value"
	}

	switch {
	case len(r.Image) == 0:
		validationErrors["image"] = "required_field"
	case strings.ContainsAny(r.Image, " \t\r\n"):
		validationErrors["image"] = "invalid_value"
	}

	for key := range r.Labels {
		if len(key) == 0 || strings.ContainsAny(key, "=\r\n") {
			validationErrors["labels"] = "invalid_value"
		}
	}

	for _, variable := range r.Env {
		if name, _, _ := strings.Cut(variable, "="); len(name) == 0 {
			validationErrors["env"] = "invalid_value"
		}
	}

	if len(r.WorkingDir) > 0 && !path.IsAbs(r.WorkingDir) {
		validationErrors["working_dir"] = "invalid_value"
	}

	if r.Resources.CPU < 0 || math.IsNaN(r.Resources.CPU) || math.IsInf(r.Resources.CPU, 0) {
		validationErrors["resources.cpu"] = "invalid_value"
	}

	if !vm.IsRestartPolicy(r.RestartPolicy) {
		validationErrors["restart_policy"] = "invalid_value"
	}

	if code, refused := validateNetworks(r.Networks); refused {
		validationErrors["networks"] = code
	}

	switch {
	case len(r.ExposedPorts) > maxExposedPorts:
		validationErrors["exposed_ports"] = "exceeds_limit"
	default:
		for _, port := range r.ExposedPorts {
			if port == 0 {
				validationErrors["exposed_ports"] = "invalid_value"
			}
		}
	}

	return validationErrors
}

// validateNetworks checks the networks a VM joins: not too many, each by a
// name a network can have, none twice, with names its neighbours can reach it
// by, and at most one it routes out through.
func validateNetworks(attachments []vm.Attachment) (string, bool) {
	if len(attachments) > maxNetworks {
		return "exceeds_limit", true
	}

	seen := make(map[string]bool, len(attachments))
	gateways := 0

	for _, attachment := range attachments {
		if !vm.IsNetworkName(attachment.Network) || seen[attachment.Network] {
			return "invalid_value", true
		}

		seen[attachment.Network] = true

		for _, alias := range attachment.Aliases {
			if !aliasPattern.MatchString(alias) {
				return "invalid_value", true
			}
		}

		if attachment.Gateway {
			gateways++
		}
	}

	if gateways > 1 {
		return "invalid_value", true
	}

	return "", false
}
