package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// project is a compose file as compose reads it, as far as these tests look:
// its services' labels, map and list alike, and what else they say.
type project struct {
	Services map[string]struct {
		Image  string         `yaml:"image"`
		Labels yaml.Node      `yaml:"labels"`
		Rest   map[string]any `yaml:",inline"`
	} `yaml:"services"`
	Rest map[string]any `yaml:",inline"`
}

// labelsOf are a service's labels as compose reads them, whichever way they
// are written: a map, or a list of key=value.
func labelsOf(t *testing.T, node yaml.Node) map[string]string {
	t.Helper()

	labels := map[string]string{}

	switch node.Kind {
	case yaml.MappingNode:
		require.NoError(t, node.Decode(&labels))
	case yaml.SequenceNode:
		var list []string
		require.NoError(t, node.Decode(&list))

		for _, item := range list {
			key, value, _ := cut(item)
			labels[key] = value
		}
	case 0:
	default:
		t.Fatalf("labels are neither a map nor a list: %v", node.Kind)
	}

	return labels
}

func cut(item string) (string, string, bool) {
	for i := range len(item) {
		if item[i] == '=' {
			return item[:i], item[i+1:], true
		}
	}

	return item, "", false
}

func read(t *testing.T, compose string) project {
	t.Helper()

	var p project
	require.NoError(t, yaml.Unmarshal([]byte(compose), &p))

	return p
}

func TestLabelled(t *testing.T) {
	t.Parallel()

	ours := func(services string) map[string]string {
		return map[string]string{"workload.stack": "stack-uuid", "workload.stack.services": services}
	}

	with := func(labels map[string]string, more map[string]string) map[string]string {
		all := map[string]string{}
		for k, v := range labels {
			all[k] = v
		}

		for k, v := range more {
			all[k] = v
		}

		return all
	}

	for name, tt := range map[string]struct {
		compose string
		labels  map[string]map[string]string
		list    []string
		plain   string
	}{
		"a service without labels is given the stack's": {
			compose: "services:\n  web:\n    image: nginx:alpine\n",
			labels:  map[string]map[string]string{"web": ours("web")},
		},
		"labels written as a map are kept, beside the stack's": {
			compose: "services:\n  web:\n    image: nginx:alpine\n    labels:\n      team: shop\n      com.example.tier: front\n",
			labels:  map[string]map[string]string{"web": with(ours("web"), map[string]string{"team": "shop", "com.example.tier": "front"})},
		},
		"labels written as a list are kept, and stay a list": {
			compose: "services:\n  web:\n    image: nginx:alpine\n    labels:\n      - team=shop\n      - flag\n",
			labels:  map[string]map[string]string{"web": with(ours("web"), map[string]string{"team": "shop", "flag": ""})},
			list:    []string{"web"},
		},
		"a label of the stack's a service had already is the stack's, in either form": {
			compose: "services:\n  web:\n    image: nginx:alpine\n    labels:\n      workload.stack: somebody-else\n  api:\n    image: nginx:alpine\n    labels: [\"workload.stack=somebody-else\", \"workload.stack.services\"]\n",
			labels:  map[string]map[string]string{"web": ours("api,web"), "api": ours("api,web")},
			list:    []string{"api"},
		},
		"an empty labels key is a map of the stack's": {
			compose: "services:\n  web:\n    image: nginx:alpine\n    labels:\n",
			labels:  map[string]map[string]string{"web": ours("web")},
		},
		"labels shared through an anchor are copied, and the anchor's other users keep theirs": {
			compose: "x-labels: &labels\n  team: shop\nservices:\n  web:\n    image: nginx:alpine\n    labels: *labels\n  api:\n    image: nginx:alpine\n    labels: *labels\n",
			labels: map[string]map[string]string{
				"web": with(ours("api,web"), map[string]string{"team": "shop"}),
				"api": with(ours("api,web"), map[string]string{"team": "shop"}),
			},
		},
		"labels a merge key brings in are copied in before the stack's are added": {
			compose: "x-defaults: &defaults\n  restart: unless-stopped\n  labels:\n    team: shop\nservices:\n  web:\n    <<: *defaults\n    image: nginx:alpine\n",
			labels:  map[string]map[string]string{"web": with(ours("web"), map[string]string{"team": "shop"})},
			plain:   "<<: *defaults",
		},
		"a whole service shared through an anchor is a copy of its own": {
			compose: "x-web: &web\n  image: nginx:alpine\nservices:\n  web: *web\n  web2: *web\n",
			labels:  map[string]map[string]string{"web": ours("web,web2"), "web2": ours("web,web2")},
		},
		"services that are not to be running are labelled, and not counted among those that are": {
			compose: "services:\n" +
				"  web:\n    image: nginx:alpine\n    depends_on:\n      migrate:\n        condition: service_completed_successfully\n      cache:\n        condition: service_started\n" +
				"  migrate:\n    image: migrate\n" +
				"  cache:\n    image: redis:alpine\n" +
				"  debug:\n    image: busybox\n    profiles: [debug]\n" +
				"  spare:\n    image: nginx:alpine\n    scale: 0\n" +
				"  replica:\n    image: nginx:alpine\n    deploy:\n      replicas: 0\n",
			labels: map[string]map[string]string{
				"web":     ours("cache,web"),
				"migrate": ours("cache,web"),
				"cache":   ours("cache,web"),
				"debug":   ours("cache,web"),
				"spare":   ours("cache,web"),
				"replica": ours("cache,web"),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			labelled, err := Labelled(tt.compose, "stack-uuid")
			require.NoError(t, err)

			p := read(t, labelled)
			require.Len(t, p.Services, len(tt.labels), "%s", labelled)

			for service, want := range tt.labels {
				assert.Equal(t, want, labelsOf(t, p.Services[service].Labels), "%s:\n%s", service, labelled)
				assert.NotEmpty(t, p.Services[service].Image, "what else the service says is kept")
			}

			for _, service := range tt.list {
				assert.Equal(t, yaml.SequenceNode, p.Services[service].Labels.Kind, "a list stays a list")
			}

			if len(tt.plain) > 0 {
				assert.Contains(t, labelled, tt.plain, "written as it usually is")
			}

			again, err := Labelled(labelled, "stack-uuid")
			require.NoError(t, err)
			assert.Equal(t, labelled, again, "labelling it again changes nothing")
		})
	}

	t.Run("what the file says besides is kept, comments and all", func(t *testing.T) {
		t.Parallel()

		compose := "# the shop\nname: ignored\nservices:\n  web:\n    image: nginx:alpine # pinned later\n    ports: [\"80:80\"]\nvolumes:\n  data: {}\n"

		labelled, err := Labelled(compose, "stack-uuid")
		require.NoError(t, err)

		assert.Contains(t, labelled, "# the shop")
		assert.Contains(t, labelled, "# pinned later")

		p := read(t, labelled)
		assert.Equal(t, "ignored", p.Rest["name"])
		assert.Contains(t, p.Rest, "volumes")
		assert.Equal(t, []any{"80:80"}, p.Services["web"].Rest["ports"])
	})

	t.Run("an anchor's other users are not changed by a copy of it", func(t *testing.T) {
		t.Parallel()

		compose := "x-labels: &labels\n  team: shop\nx-elsewhere:\n  labels: *labels\nservices:\n  web:\n    image: nginx:alpine\n    labels: *labels\n"

		labelled, err := Labelled(compose, "stack-uuid")
		require.NoError(t, err)

		var whole struct {
			Labels    map[string]string `yaml:"x-labels"`
			Elsewhere struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"x-elsewhere"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(labelled), &whole))

		assert.Equal(t, map[string]string{"team": "shop"}, whole.Labels)
		assert.Equal(t, map[string]string{"team": "shop"}, whole.Elsewhere.Labels)
	})

	for name, compose := range map[string]string{
		"one that is not YAML":                 "services: [",
		"one that is empty":                    "",
		"one that is not a mapping":            "- web\n",
		"one with no services":                 "volumes:\n  data: {}\n",
		"one whose services are not a mapping": "services: [web]\n",
		"one with a service that is a list":    "services:\n  web: [nginx]\n",
		"one whose labels are a word":          "services:\n  web:\n    image: nginx\n    labels: shop\n",
	} {
		t.Run(name+" is not labelled", func(t *testing.T) {
			t.Parallel()

			_, err := Labelled(compose, "stack-uuid")
			assert.Error(t, err)
		})
	}
}
