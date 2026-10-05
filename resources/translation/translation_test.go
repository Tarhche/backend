package translation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEveryLanguageSaysEverything holds the languages to one another: a code
// one of them has no words for reaches its readers as an empty message, which
// says less than the code itself.
func TestEveryLanguageSaysEverything(t *testing.T) {
	t.Parallel()

	for locale, words := range Translations {
		for other, otherWords := range Translations {
			for key := range otherWords {
				assert.Contains(t, words, key, "%s says %q and %s does not", other, key, locale)
			}
		}

		for key, said := range words {
			assert.NotEmpty(t, said, "%s says %q as nothing", locale, key)
		}
	}
}

// TestTheWorkloadsCodesAreSaid holds the codes the workload answers with to
// having words: the dashboard's own rules, the control plane's refusals and a
// node's, which reach a reader as they are said here.
func TestTheWorkloadsCodesAreSaid(t *testing.T) {
	t.Parallel()

	codes := []string{
		// the dashboard's
		"invalid_kind", "invalid_access", "invalid_port", "duplicate_port", "too_many_ports",
		"invalid_lifetime", "invalid_image", "invalid_name", "invalid_protocol",
		"invalid_mount_type", "invalid_mount", "invalid_restart_policy",
		"invalid_network_driver", "invalid_volume_driver", "vm_or_new_vm",

		// the control plane's
		"vm_required", "too_large", "too_small", "quota_exceeded", "no_capacity",
		"engine_mismatch", "kind_mismatch", "snapshot_not_ready", "disk_too_small",
		"disk_cannot_shrink", "immutable", "vm_not_running", "node_lost",

		// a node's
		"not_found", "not_running", "not_docker", "docker_unavailable", "invalid", "timeout", "internal",
	}

	for locale, words := range Translations {
		for _, code := range codes {
			assert.NotEmpty(t, words[code], "%s has no words for %q", locale, code)
		}
	}
}
