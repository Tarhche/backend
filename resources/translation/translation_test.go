package translation

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTranslations holds the messages to what the validator does with them: a
// code it is given no words for comes out as an empty message, which is a
// refusal the reader cannot read.
func TestTranslations(t *testing.T) {
	t.Parallel()

	t.Run("every message is said in every language", func(t *testing.T) {
		t.Parallel()

		english := slices.Sorted(maps.Keys(Translations[EN]))

		for language, messages := range Translations {
			assert.Equal(t, english, slices.Sorted(maps.Keys(messages)), language)

			for key, message := range messages {
				assert.NotEmpty(t, message, "%s says nothing for %q", language, key)
			}
		}
	})

	t.Run("what the workload refuses a task or a stack for has words", func(t *testing.T) {
		t.Parallel()

		codes := []string{
			"required_field",
			"invalid_value",
			"invalid_network_policy",
			"ports_require_network",
			"memory_below_minimum",
			"not_supported",
			"too_many_services",
			"mixed_runtimes_in_stack",
			"ttl_requires_a_job",
			"task_is_not_terminal_state",
			"too_many",
		}

		for _, code := range codes {
			assert.Contains(t, Translations[EN], code)
			assert.Contains(t, Translations[FA], code)
		}
	})
}
