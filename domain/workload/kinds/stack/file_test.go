package stack_test

import (
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

// everything is a file that says every key a stack's file takes, each form of
// each among it.
const everything = `
services:
  db:
    image: postgres:17
    environment:
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: shop
      EMPTY:
      PORT: 5432
    volumes:
      - data:/var/lib/postgresql/data
      - type: bind
        source: /srv/initdb
        target: /docker-entrypoint-initdb.d
        read_only: true
    networks:
      - backend
    restart: unless-stopped
    pids_limit: 100
    resources:
      limits:
        cpus: 0.5
        memory: 512m
  web:
    image: nginx:1.27
    command: nginx -g 'daemon off;' "a \"b\""
    entrypoint: ["/docker-entrypoint.sh", "--"]
    environment:
      - MODE=production
      - URL=https://shop.example.com/?a=b
    ports:
      - "80"
      - "8080:80"
      - "127.0.0.1:8443:443/tcp"
      - "127.0.0.1::9000"
      - "[::1]:5353:53/udp"
      - "9090-9091:8080-8081"
      - target: 3000
        published: 3001
        host_ip: 0.0.0.0
        protocol: udp
      - target: 4000
    privileged: true
    restart: "no"
    depends_on:
      db:
        condition: service_started
      cache: {}
    volumes:
      - data:/data:ro
      - /srv/static:/usr/share/nginx/html:rw
      - type: volume
        source: data
        target: /backup
    networks:
      frontend:
      backend: {}
    user: "1000:1000"
    runtime: runc
    resources:
      limits:
        cpus: "2"
        memory: 1073741824
  cache:
    image: redis:7
    command: [redis-server, --save, "60", "1"]
    depends_on: [db]
volumes:
  data:
networks:
  frontend: {}
  backend: ~
`

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("a file that says every key it takes, in each of its forms", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse(everything)
		require.Empty(t, invalid)

		assert.Equal(t, stack.File{
			Services: map[string]stack.Service{
				"db": {
					Image:       "postgres:17",
					Environment: []string{"POSTGRES_PASSWORD=secret", "POSTGRES_DB=shop", "EMPTY=", "PORT=5432"},
					Restart:     stack.RestartUnlessStopped,
					Volumes: []stack.Mount{
						{Type: stack.MountVolume, Source: "data", Target: "/var/lib/postgresql/data"},
						{Type: stack.MountBind, Source: "/srv/initdb", Target: "/docker-entrypoint-initdb.d", ReadOnly: true},
					},
					Networks:  []string{"backend"},
					PidsLimit: 100,
					CPUs:      0.5,
					Memory:    512 << 20,
				},
				"web": {
					Image:       "nginx:1.27",
					Command:     []string{"nginx", "-g", "daemon off;", `a "b"`},
					Entrypoint:  []string{"/docker-entrypoint.sh", "--"},
					Environment: []string{"MODE=production", "URL=https://shop.example.com/?a=b"},
					Ports: []stack.Port{
						{Container: 80, Protocol: "tcp"},
						{Container: 80, Host: 8080, Protocol: "tcp"},
						{Container: 443, Host: 8443, HostIP: "127.0.0.1", Protocol: "tcp"},
						{Container: 9000, HostIP: "127.0.0.1", Protocol: "tcp"},
						{Container: 53, Host: 5353, HostIP: "::1", Protocol: "udp"},
						{Container: 8080, Host: 9090, Protocol: "tcp"},
						{Container: 8081, Host: 9091, Protocol: "tcp"},
						{Container: 3000, Host: 3001, HostIP: "0.0.0.0", Protocol: "udp"},
						{Container: 4000, Protocol: "tcp"},
					},
					Privileged: true,
					Restart:    stack.RestartNo,
					DependsOn:  []string{"cache", "db"},
					Volumes: []stack.Mount{
						{Type: stack.MountVolume, Source: "data", Target: "/data", ReadOnly: true},
						{Type: stack.MountBind, Source: "/srv/static", Target: "/usr/share/nginx/html"},
						{Type: stack.MountVolume, Source: "data", Target: "/backup"},
					},
					Networks: []string{"backend", "frontend"},
					User:     "1000:1000",
					Runtime:  "runc",
					CPUs:     2,
					Memory:   1 << 30,
				},
				"cache": {
					Image:     "redis:7",
					Command:   []string{"redis-server", "--save", "60", "1"},
					Restart:   stack.RestartNo,
					DependsOn: []string{"db"},
					Networks:  []string{stack.DefaultNetwork},
				},
			},
			Volumes:  []string{"data"},
			Networks: []string{"backend", "frontend"},
		}, file)

		assert.Equal(t, []string{"cache", "db", "web"}, file.ServiceNames())
		assert.Equal(t, []string{"backend", "default", "frontend"}, file.NetworksMade(), "what it declares, and the default one a service joins by naming none")
		assert.Equal(t, [][]string{{"db"}, {"cache"}, {"web"}}, file.Levels(), "each service after what it depends on")
	})

	t.Run("a value is what it says, a dollar among it: nothing is interpolated", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse(`
services:
  web:
    image: nginx:${TAG}
    command: echo ${HOME} '$PATH' "$$"
    environment:
      HOME: ${HOME:-/root}
      PASSWORD: $$secret
`)
		require.Empty(t, invalid)

		web := file.Services["web"]
		assert.Equal(t, "nginx:${TAG}", web.Image)
		assert.Equal(t, []string{"echo", "${HOME}", "$PATH", "$$"}, web.Command)
		assert.Equal(t, []string{"HOME=${HOME:-/root}", "PASSWORD=$$secret"}, web.Environment)
	})

	t.Run("a service that names no network joins the default one, which is made only then", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse("services:\n  web:\n    image: nginx\n    networks: [front]\nnetworks:\n  front:\n  back:\n")
		require.Empty(t, invalid)
		assert.Equal(t, []string{"back", "front"}, file.NetworksMade(), "every one it declares, and no default one nothing joins")

		file, invalid = stack.Parse("services:\n  web:\n    image: nginx\n  db:\n    image: postgres\n    networks: {default: }\n")
		require.Empty(t, invalid)
		assert.Equal(t, []string{"default"}, file.Services["web"].Networks)
		assert.Equal(t, []string{"default"}, file.Services["db"].Networks, "which it may name without declaring it")
		assert.Equal(t, []string{"default"}, file.NetworksMade())
	})

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"nothing": {
			file:    " \n",
			invalid: domain.ValidationErrors{"compose": "required_field"},
		},
		"what is not YAML": {
			file:    "nope: [",
			invalid: domain.ValidationErrors{"compose": "invalid_value"},
		},
		"a list rather than a mapping": {
			file:    "- web\n",
			invalid: domain.ValidationErrors{"compose": "invalid_value"},
		},
		"two documents": {
			file:    "services:\n  web:\n    image: nginx\n---\nservices:\n  db:\n    image: postgres\n",
			invalid: domain.ValidationErrors{"compose": "invalid_value"},
		},
		"no services": {
			file:    "volumes:\n  data:\n",
			invalid: domain.ValidationErrors{"compose.services": "required_field"},
		},
		"services that are none": {
			file:    "services: {}\n",
			invalid: domain.ValidationErrors{"compose.services": "required_field"},
		},
		"services that are a list": {
			file:    "services: [web]\n",
			invalid: domain.ValidationErrors{"compose.services": "invalid_value"},
		},
		"a service that is not a mapping": {
			file:    "services:\n  web: nginx\n",
			invalid: domain.ValidationErrors{"compose.services.web": "invalid_value"},
		},
		"a service with no image": {
			file:    "services:\n  web:\n    command: [sleep, infinity]\n",
			invalid: domain.ValidationErrors{"compose.services.web.image": "required_field"},
		},
		"an image with a blank in it": {
			file:    "services:\n  web:\n    image: nginx latest\n",
			invalid: domain.ValidationErrors{"compose.services.web.image": "invalid_image"},
		},
		"a restart it does not carry out": {
			file:    "services:\n  web:\n    image: nginx\n    restart: always\n",
			invalid: domain.ValidationErrors{"compose.services.web.restart": "invalid_restart"},
		},
		"nor this one": {
			file:    "services:\n  web:\n    image: nginx\n    restart: on-failure\n",
			invalid: domain.ValidationErrors{"compose.services.web.restart": "invalid_restart"},
		},
		"a service it depends on that is not there": {
			file:    "services:\n  web:\n    image: nginx\n    depends_on: [db]\n",
			invalid: domain.ValidationErrors{"compose.services.web.depends_on.db": "undefined_service"},
		},
		"or that is waited on for more than having started": {
			file:    "services:\n  web:\n    image: nginx\n    depends_on:\n      db:\n        condition: service_healthy\n  db:\n    image: postgres\n",
			invalid: domain.ValidationErrors{"compose.services.web.depends_on.db.condition": "invalid_condition"},
		},
		"a volume it mounts that is not declared": {
			file:    "services:\n  web:\n    image: nginx\n    volumes: [data:/data]\n",
			invalid: domain.ValidationErrors{"compose.services.web.volumes[0]": "undefined_volume"},
		},
		"nor in its long form": {
			file:    "services:\n  web:\n    image: nginx\n    volumes:\n      - type: volume\n        source: data\n        target: /data\n",
			invalid: domain.ValidationErrors{"compose.services.web.volumes[0]": "undefined_volume"},
		},
		"a network it joins that is not declared": {
			file:    "services:\n  web:\n    image: nginx\n    networks: [front]\n",
			invalid: domain.ValidationErrors{"compose.services.web.networks.front": "undefined_network"},
		},
		"nor in a mapping": {
			file:    "services:\n  web:\n    image: nginx\n    networks:\n      front:\n",
			invalid: domain.ValidationErrors{"compose.services.web.networks.front": "undefined_network"},
		},
		"a key twice": {
			file:    "services:\n  web:\n    image: nginx\n    image: httpd\n",
			invalid: domain.ValidationErrors{"compose.services.web.image": "duplicate_key"},
		},
		"a service twice": {
			file:    "services:\n  web:\n    image: nginx\n  web:\n    image: httpd\n",
			invalid: domain.ValidationErrors{"compose.services.web": "duplicate_key"},
		},
		"a variable twice": {
			file:    "services:\n  web:\n    image: nginx\n    environment:\n      A: one\n      A: two\n",
			invalid: domain.ValidationErrors{"compose.services.web.environment.A": "duplicate_key"},
		},
		"a variable with no value in a list": {
			file:    "services:\n  web:\n    image: nginx\n    environment: [MODE]\n",
			invalid: domain.ValidationErrors{"compose.services.web.environment[0]": "invalid_value"},
		},
		"a variable that is a list": {
			file:    "services:\n  web:\n    image: nginx\n    environment:\n      MODES: [a, b]\n",
			invalid: domain.ValidationErrors{"compose.services.web.environment.MODES": "invalid_value"},
		},
		"a command whose quote is left open": {
			file:    "services:\n  web:\n    image: nginx\n    command: echo 'hello\n",
			invalid: domain.ValidationErrors{"compose.services.web.command": "invalid_value"},
		},
		"a command that is a mapping": {
			file:    "services:\n  web:\n    image: nginx\n    command: {run: it}\n",
			invalid: domain.ValidationErrors{"compose.services.web.command": "invalid_value"},
		},
		"privileged that is neither true nor false": {
			file:    "services:\n  web:\n    image: nginx\n    privileged: yes\n",
			invalid: domain.ValidationErrors{"compose.services.web.privileged": "invalid_value"},
		},
		"fewer processes than none": {
			file:    "services:\n  web:\n    image: nginx\n    pids_limit: -1\n",
			invalid: domain.ValidationErrors{"compose.services.web.pids_limit": "invalid_value"},
		},
		"resources that are not a mapping": {
			file:    "services:\n  web:\n    image: nginx\n    resources: 2\n",
			invalid: domain.ValidationErrors{"compose.services.web.resources": "invalid_value"},
		},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parse(tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}

	t.Run("refuses a file larger than a stack may be given", func(t *testing.T) {
		t.Parallel()

		_, invalid := stack.Parse("services:\n  web:\n    image: nginx\n" + strings.Repeat("#", stack.MaxFile))

		assert.Equal(t, domain.ValidationErrors{"compose": "too_large"}, invalid)
	})
}

func TestParse_unsupportedKeys(t *testing.T) {
	t.Parallel()

	service := func(key string) string {
		return "services:\n  web:\n    image: nginx\n" + key + "\n"
	}

	for name, tt := range map[string]struct {
		file string
		path string
	}{
		"a build":                       {file: service("    build: ."), path: "compose.services.web.build"},
		"a healthcheck":                 {file: service("    healthcheck:\n      test: [CMD, true]"), path: "compose.services.web.healthcheck"},
		"a service's configs":           {file: service("    configs: [site]"), path: "compose.services.web.configs"},
		"a service's secrets":           {file: service("    secrets: [key]"), path: "compose.services.web.secrets"},
		"profiles":                      {file: service("    profiles: [debug]"), path: "compose.services.web.profiles"},
		"extends":                       {file: service("    extends: base"), path: "compose.services.web.extends"},
		"deploy":                        {file: service("    deploy:\n      resources:\n        limits:\n          cpus: 1"), path: "compose.services.web.deploy"},
		"a key compose has of its own":  {file: service("    container_name: shop"), path: "compose.services.web.container_name"},
		"any other key":                 {file: service("    flavour: vanilla"), path: "compose.services.web.flavour"},
		"an x- key of a service":        {file: service("    x-team: shop"), path: "compose.services.web.x-team"},
		"a merge of another mapping":    {file: service("    <<: {user: root}"), path: "compose.services.web.<<"},
		"a top-level version":           {file: "version: '3.9'\n" + service(""), path: "compose.version"},
		"a top-level name":              {file: "name: shop\n" + service(""), path: "compose.name"},
		"a top-level x- key":            {file: "x-common: {}\n" + service(""), path: "compose.x-common"},
		"top-level configs":             {file: "configs:\n  site: {}\n" + service(""), path: "compose.configs"},
		"top-level secrets":             {file: "secrets:\n  key: {}\n" + service(""), path: "compose.secrets"},
		"reservations":                  {file: service("    resources:\n      reservations:\n        cpus: 1"), path: "compose.services.web.resources.reservations"},
		"a limit of anything else":      {file: service("    resources:\n      limits:\n        pids: 10"), path: "compose.services.web.resources.limits.pids"},
		"a port's mode":                 {file: service("    ports:\n      - target: 80\n        mode: host"), path: "compose.services.web.ports[0].mode"},
		"a volume's options":            {file: service("    volumes:\n      - type: volume\n        source: data\n        target: /data\n        volume:\n          nocopy: true"), path: "compose.services.web.volumes[0].volume"},
		"a dependency's restart":        {file: "services:\n  web:\n    image: nginx\n    depends_on:\n      db:\n        restart: true\n  db:\n    image: postgres\n", path: "compose.services.web.depends_on.db.restart"},
		"what a service says of a net":  {file: service("    networks:\n      default:\n        aliases: [www]"), path: "compose.services.web.networks.default.aliases"},
		"a volume declared with more":   {file: service("") + "volumes:\n  data:\n    driver: local\n", path: "compose.volumes.data.driver"},
		"a network declared with more":  {file: service("") + "networks:\n  front:\n    external: true\n", path: "compose.networks.front.external"},
		"a network that is no mapping":  {file: service("") + "networks: [front]\n", path: "compose.networks"},
		"a volume declared as anything": {file: service("") + "volumes:\n  data: local\n", path: "compose.volumes.data"},
	} {
		t.Run("refuses "+name+", saying where it is", func(t *testing.T) {
			t.Parallel()

			_, invalid := stack.Parse(tt.file)

			require.Contains(t, invalid, tt.path, "%v", invalid)

			if strings.HasSuffix(name, "no mapping") || strings.HasSuffix(name, "as anything") {
				assert.Equal(t, "invalid_value", invalid[tt.path])
			} else {
				assert.Equal(t, stack.CodeUnsupportedKey, invalid[tt.path])
			}
		})
	}
}

func TestParse_aliases(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"a service anchored, and another that is an alias of it": {
			file:    "services:\n  web: &web\n    image: nginx\n  api: *web\n",
			invalid: domain.ValidationErrors{"compose.services.web": stack.CodeUnsupportedAlias, "compose.services.api": stack.CodeUnsupportedAlias},
		},
		"an anchor nothing is an alias of": {
			file:    "services:\n  web:\n    image: &image nginx\n",
			invalid: domain.ValidationErrors{"compose.services.web.image": stack.CodeUnsupportedAlias},
		},
		"a merge of what an x- key anchors": {
			file:    "x-base: &base\n  image: nginx\nservices:\n  web:\n    <<: *base\n",
			invalid: domain.ValidationErrors{"compose.x-base": stack.CodeUnsupportedAlias, "compose.services.web.<<": stack.CodeUnsupportedAlias},
		},
		"an environment anchored, and another service's alias of it": {
			file:    "services:\n  web:\n    image: nginx\n    environment: &env {MODE: production}\n  api:\n    image: api\n    environment: *env\n",
			invalid: domain.ValidationErrors{"compose.services.web.environment": stack.CodeUnsupportedAlias, "compose.services.api.environment": stack.CodeUnsupportedAlias},
		},
		"an item of a list, and an alias of it beside it": {
			file:    "services:\n  web:\n    image: nginx\n    ports: [&port \"80\", *port]\n",
			invalid: domain.ValidationErrors{"compose.services.web.ports[0]": stack.CodeUnsupportedAlias, "compose.services.web.ports[1]": stack.CodeUnsupportedAlias},
		},
		"a key anchored": {
			file:    "services:\n  &name web:\n    image: nginx\n",
			invalid: domain.ValidationErrors{"compose.services.web": stack.CodeUnsupportedAlias},
		},
		"the file itself anchored": {
			file:    "--- &file\nservices:\n  web:\n    image: nginx\n",
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedAlias},
		},
		"before anything else of the file is read": {
			file:    "version: '3'\nservices:\n  web:\n    image: nginx latest\n    build: .\n    user: &user root\n",
			invalid: domain.ValidationErrors{"compose.services.web.user": stack.CodeUnsupportedAlias},
		},
	} {
		t.Run("refuses "+name+", saying where", func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parse(tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}
}

func TestParse_depth(t *testing.T) {
	t.Parallel()

	// listed is a file whose service's command is lists, each the one item of
	// the one around it, as many as there are, the innermost holding inner:
	// the command is three levels beneath the file's top, services one and
	// web two, so inner is as many levels beneath it as there are lists, and
	// three more.
	listed := func(lists int, inner string) string {
		return "services:\n  web:\n    image: nginx\n    command: " + strings.Repeat("[", lists) + inner + strings.Repeat("]", lists) + "\n"
	}

	// mapped is the same of mappings, each keyed k.
	mapped := func(mappings int, inner string) string {
		return "services:\n  web:\n    image: nginx\n    command: " + strings.Repeat("{k: ", mappings) + inner + strings.Repeat("}", mappings) + "\n"
	}

	command := "compose.services.web.command"

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"what is as deep as a file nests is read, a command of lists, which is refused for what it is": {
			file:    listed(stack.MaxDepth-3, "a"),
			invalid: domain.ValidationErrors{command + "[0]": "invalid_value"},
		},
		"and a level deeper is too deep, under the path of the first of it": {
			file:    listed(stack.MaxDepth-2, "a"),
			invalid: domain.ValidationErrors{command + strings.Repeat("[0]", stack.MaxDepth-2): stack.CodeTooDeep},
		},
		"and what is beneath that is not read, an anchor among it": {
			file:    listed(9990, "&x a"),
			invalid: domain.ValidationErrors{command + strings.Repeat("[0]", stack.MaxDepth-2): stack.CodeTooDeep},
		},
		"nor what is beside it in what holds it": {
			file:    listed(stack.MaxDepth-3, "[a, &x b, [c]]"),
			invalid: domain.ValidationErrors{command + strings.Repeat("[0]", stack.MaxDepth-2): stack.CodeTooDeep},
		},
		"but an anchor above it is said, before it": {
			file: "services:\n  web:\n    image: &i nginx\n    command: " + strings.Repeat("[", 20) + "a" + strings.Repeat("]", 20) + "\n",
			invalid: domain.ValidationErrors{
				"compose.services.web.image":                      stack.CodeUnsupportedAlias,
				command + strings.Repeat("[0]", stack.MaxDepth-2): stack.CodeTooDeep,
			},
		},
		"mappings as deep as a file nests are read": {
			file:    mapped(stack.MaxDepth-3, "a"),
			invalid: domain.ValidationErrors{command: "invalid_value"},
		},
		"and a level deeper is too deep, under the path of its key": {
			file:    mapped(stack.MaxDepth-2, "a"),
			invalid: domain.ValidationErrors{command + strings.Repeat(".k", stack.MaxDepth-2): stack.CodeTooDeep},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parse(tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}
}

func TestParse_caps(t *testing.T) {
	t.Parallel()

	// services are n services, s0 first; and declared is a service, and n
	// volumes or networks the file declares under key.
	services := func(n int) string {
		var b strings.Builder

		b.WriteString("services:\n")
		for i := range n {
			fmt.Fprintf(&b, "  s%d: {image: nginx}\n", i)
		}

		return b.String()
	}

	declared := func(key string, n int) string {
		var b strings.Builder

		b.WriteString("services:\n  web: {image: nginx}\n" + key + ":\n")
		for i := range n {
			fmt.Fprintf(&b, "  d%d:\n", i)
		}

		return b.String()
	}

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"as many services as a file may have":     {file: services(stack.MaxServices)},
		"one more is too many":                    {file: services(stack.MaxServices + 1), invalid: domain.ValidationErrors{"compose.services": "too_large"}},
		"as many volumes as it may declare":       {file: declared("volumes", stack.MaxVolumes)},
		"one more volume is too many":             {file: declared("volumes", stack.MaxVolumes+1), invalid: domain.ValidationErrors{"compose.volumes": "too_large"}},
		"as many networks as it may declare":      {file: declared("networks", stack.MaxNetworks)},
		"one more network is too many":            {file: declared("networks", stack.MaxNetworks+1), invalid: domain.ValidationErrors{"compose.networks": "too_large"}},
		"as many ports as its services may share": {file: "services:\n  web:\n    image: nginx\n    ports: [1-600]\n  db:\n    image: postgres\n    ports: [1-399, {target: 5432}]\n"},
		"and one more, refused where it is, the services read in the order the file says them, and no port after it read": {
			file:    "services:\n  web:\n    image: nginx\n    ports: [1-600]\n  db:\n    image: postgres\n    ports: [1-400, \"5432\", nope]\n",
			invalid: domain.ValidationErrors{"compose.services.db.ports[1]": "too_large"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parse(tt.file)

			assert.Equal(t, tt.invalid, invalid)

			if len(tt.invalid) > 0 {
				assert.Empty(t, file.Services, "and is no file")

				return
			}

			published := 0
			for _, s := range file.Services {
				published += len(s.Ports)
			}

			assert.LessOrEqual(t, published, stack.MaxPorts)
			assert.NotEmpty(t, file.Services)
		})
	}

	t.Run("a range is counted by its size before any port of it is, all of a file's ports at most", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse("services:\n  web:\n    image: nginx\n    ports: [\"1-1000:1-1000\"]\n")
		require.Empty(t, invalid)

		ports := file.Services["web"].Ports
		require.Len(t, ports, stack.MaxPorts)
		assert.Equal(t, stack.Port{Container: 1000, Host: 1000, Protocol: "tcp"}, ports[stack.MaxPorts-1])

		_, invalid = stack.Parse("services:\n  web:\n    image: nginx\n    ports: [\"1-65535\"]\n")
		assert.Equal(t, domain.ValidationErrors{"compose.services.web.ports[0]": "too_large"}, invalid)
	})
}

func TestParse_mostInvalid(t *testing.T) {
	t.Parallel()

	t.Run("what is wrong is said of the first a file says, as many as it ever says, and of nothing after", func(t *testing.T) {
		t.Parallel()

		var b strings.Builder

		b.WriteString("services:\n  web:\n    image: nginx\n")
		for i := range stack.MaxInvalid + 50 {
			fmt.Fprintf(&b, "    k%03d: 1\n", i)
		}

		_, invalid := stack.Parse(b.String())
		require.Len(t, invalid, stack.MaxInvalid)

		var want []string
		for i := range stack.MaxInvalid {
			want = append(want, fmt.Sprintf("compose.services.web.k%03d", i))
		}

		assert.Equal(t, want, slices.Sorted(maps.Keys(invalid)))
	})

	t.Run("its services in the order the file says them, not of their names", func(t *testing.T) {
		t.Parallel()

		var b strings.Builder

		b.WriteString("services:\n")
		for i := stack.MaxServices - 1; i >= 0; i-- {
			fmt.Fprintf(&b, "  s%02d: {image: nginx latest, restart: always}\n", i)
		}

		_, invalid := stack.Parse(b.String())
		require.Len(t, invalid, stack.MaxInvalid)

		for i := range stack.MaxServices {
			said := stack.MaxServices-i <= stack.MaxInvalid/2

			for key, code := range map[string]string{"image": "invalid_image", "restart": stack.CodeInvalidRestart} {
				path := fmt.Sprintf("compose.services.s%02d.%s", i, key)

				if said {
					assert.Equal(t, code, invalid[path], path)
				} else {
					assert.NotContains(t, invalid, path)
				}
			}
		}
	})

	t.Run("and the keys of a service in the order it says them", func(t *testing.T) {
		t.Parallel()

		var b strings.Builder

		b.WriteString("services:\n  web:\n    restart: always\n    environment:\n")
		for range stack.MaxInvalid - 1 {
			b.WriteString("      - MODE\n")
		}
		b.WriteString("    image: nginx latest\n")

		_, invalid := stack.Parse(b.String())
		require.Len(t, invalid, stack.MaxInvalid)

		assert.Equal(t, stack.CodeInvalidRestart, invalid["compose.services.web.restart"])
		assert.Equal(t, "invalid_value", invalid[fmt.Sprintf("compose.services.web.environment[%d]", stack.MaxInvalid-2)])
		assert.NotContains(t, invalid, "compose.services.web.image", "which comes after the most that is said")
	})
}

// TestParse_bounded holds what reading a stack's file takes to what the file
// is, however it is written: each file a review found taking up to a
// gigabyte to read, by aliases and ranges of ports, and up to seventeen, by
// the paths of what is beneath a long name or nested deep, each written out
// whether it was refused or not, is refused or read taking a few megabytes, or
// a few tens of them where as much is said of it as ever is; and a file as
// large as a stack's may be, as costly as the bounds let one be, is read in a
// few tens of them.
//
// It runs alone, not in parallel, since what other tests allocated meanwhile
// would be counted with what reading the files does.
func TestParse_bounded(t *testing.T) {
	const megabyte = 1 << 20

	for name, tt := range map[string]struct {
		file    string
		invalid func(t *testing.T, invalid domain.ValidationErrors)
		most    uint64
	}{
		"ports anchored on one service, a hundred ranges of a thousand, and aliased by thirty more": {
			file: func() string {
				var b strings.Builder

				b.WriteString("services:\n  s0:\n    image: a\n    ports: &p\n" + strings.Repeat("      - 1-1000\n", 100))
				for i := 1; i <= 30; i++ {
					fmt.Fprintf(&b, "  s%d: {image: a, ports: *p}\n", i)
				}

				return b.String()
			}(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				want := domain.ValidationErrors{}
				for i := 0; i <= 30; i++ {
					want[fmt.Sprintf("compose.services.s%d.ports", i)] = stack.CodeUnsupportedAlias
				}

				assert.Equal(t, want, invalid)
			},
			most: 4 * megabyte,
		},
		"64 KiB of ranges of a thousand ports, no alias among them": {
			file: ranges(64 << 10),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services.web.ports[1]": "too_large"}, invalid)
			},
			most: 8 * megabyte,
		},
		"as many of them as a file may be": {
			file: ranges(stack.MaxFile),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services.web.ports[1]": "too_large"}, invalid)
			},
			most: 16 * megabyte,
		},
		"a service anchored with a thousand keys it does not take, aliased by two thousand": {
			file: func() string {
				var b strings.Builder

				b.WriteString("services:\n  s0: &s\n    image: a\n")
				for i := range 1000 {
					fmt.Fprintf(&b, "    k%d: 1\n", i)
				}

				for i := 1; i <= 2000; i++ {
					fmt.Fprintf(&b, "  s%d: *s\n", i)
				}

				return b.String()
			}(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				want := domain.ValidationErrors{}
				for i := range stack.MaxInvalid {
					want[fmt.Sprintf("compose.services.s%d", i)] = stack.CodeUnsupportedAlias
				}

				assert.Equal(t, want, invalid, "the first of them, and no more")
			},
			most: 4 * megabyte,
		},
		"an environment anchored on one service, and aliased by every other": {
			file: func() string {
				var b strings.Builder

				b.WriteString("services:\n  s0:\n    image: a\n    environment: &e\n")
				for i := range 2000 {
					fmt.Fprintf(&b, "      K%d: v\n", i)
				}

				for i := 1; b.Len() < stack.MaxFile-40 && i <= 2000; i++ {
					fmt.Fprintf(&b, "  s%d: {image: a, environment: *e}\n", i)
				}

				return b.String()
			}(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				want := domain.ValidationErrors{}
				for i := range stack.MaxInvalid {
					want[fmt.Sprintf("compose.services.s%d.environment", i)] = stack.CodeUnsupportedAlias
				}

				assert.Equal(t, want, invalid)
			},
			most: 8 * megabyte,
		},
		"a file as large as one may be, as costly to read as one can be: as many services as it may have, and each word of their commands a node of its own": {
			file: costliest(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 64 * megabyte,
		},
		"a service named by 128 KiB, its command as many words as fit, the last of them anchored, which is said under its path once": {
			file: anchoredBeneath(128<<10, 0),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				require.Len(t, invalid, 1)

				for path, code := range invalid {
					assert.True(t, strings.HasPrefix(path, "compose.services."+strings.Repeat("a", 128<<10)+".command["), "%.40s...", path)
					assert.Equal(t, stack.CodeUnsupportedAlias, code)
				}
			},
			most: 32 * megabyte,
		},
		"and with a hundred and one anchored before the rest, which are said as many as are ever said, and nothing is read after them": {
			file: anchoredBeneath(128<<10, stack.MaxInvalid+1),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Len(t, invalid, stack.MaxInvalid)

				for path, code := range invalid {
					assert.Equal(t, stack.CodeUnsupportedAlias, code, "%.40s...", path)
				}
			},
			most: 64 * megabyte,
		},
		"a list nested 9990 deep, an anchor at its bottom, which is too deep to be read": {
			file: "services:\n  web:\n    image: a\n    command: " + strings.Repeat("[", 9990) + "&x a" + strings.Repeat("]", 9990) + "\n",
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services.web.command" + strings.Repeat("[0]", stack.MaxDepth-2): stack.CodeTooDeep}, invalid)
			},
			most: 16 * megabyte,
		},
		"mappings nested as deep as a file holds of them, of keys of twenty letters, an anchor at their bottom": {
			file: func() string {
				var b strings.Builder

				b.WriteString("services:\n  web:\n    image: a\n    command: ")

				key, depth := strings.Repeat("k", 20), 0
				for ; b.Len()+len(key)+len("{: &x a}\n")+depth < stack.MaxFile && depth < 9990; depth++ {
					b.WriteString("{" + key + ": ")
				}

				b.WriteString("&x a" + strings.Repeat("}", depth) + "\n")

				return b.String()
			}(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services.web.command" + strings.Repeat("."+strings.Repeat("k", 20), stack.MaxDepth-2): stack.CodeTooDeep}, invalid)
			},
			most: 16 * megabyte,
		},
		"a service named by 128 KiB over as many binds as fit, whose name is refused once, and nothing of it read": {
			file: boundMounts(128 << 10),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + strings.Repeat("a", 128<<10): "too_large"}, invalid)
			},
			most: 16 * megabyte,
		},
		"a service named as long as a name may be over as many binds as fit, each path of which is written out only when it is refused": {
			file: boundMounts(stack.MaxNameLength),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 32 * megabyte,
		},
		"and over as many variables as fit": {
			file: func() string {
				var b strings.Builder

				b.WriteString("services:\n  ? " + strings.Repeat("a", stack.MaxNameLength) + "\n  : image: a\n    environment: {")
				for i := 0; b.Len()+len(",K00000: v}\n") <= stack.MaxFile; i++ {
					if i > 0 {
						b.WriteString(",")
					}

					fmt.Fprintf(&b, "K%x: v", i)
				}

				b.WriteString("}\n")

				return b.String()
			}(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 32 * megabyte,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.LessOrEqual(t, len(tt.file), stack.MaxFile)

			var before, after runtime.MemStats

			runtime.GC()
			runtime.ReadMemStats(&before)

			_, invalid := stack.Parse(tt.file)

			runtime.ReadMemStats(&after)

			tt.invalid(t, invalid)

			allocated := after.TotalAlloc - before.TotalAlloc
			assert.Less(t, allocated, tt.most, "%d MiB allocated reading %d KiB", allocated/megabyte, len(tt.file)>>10)
		})
	}
}

// ranges is a file of one service whose ports are ranges of a thousand each,
// as many as size bytes hold.
func ranges(size int) string {
	var b strings.Builder

	b.WriteString("services:\n  web:\n    image: a\n    ports:\n")
	for b.Len()+len("      - 1-1000\n") <= size {
		b.WriteString("      - 1-1000\n")
	}

	return b.String()
}

// anchoredBeneath is a file as large as a stack's may be of one service named
// by as many letters as length says, an explicit key's, which YAML holds to
// no length, whose command is as many one-letter words as fit: the first
// anchored ones anchored each, and the last of them anchored too.
func anchoredBeneath(length int, anchored int) string {
	var b strings.Builder

	b.WriteString("services:\n  ? " + strings.Repeat("a", length) + "\n  : image: a\n    command: [")
	for i := range anchored {
		fmt.Fprintf(&b, "&x%d a,", i)
	}

	for b.Len()+len("a,&z a]\n") <= stack.MaxFile {
		b.WriteString("a,")
	}

	b.WriteString("&z a]\n")

	return b.String()
}

// boundMounts is a file as large as a stack's may be of one service named by
// as many letters as length says, an explicit key's, which mounts as many
// binds as fit.
func boundMounts(length int) string {
	var b strings.Builder

	b.WriteString("services:\n  ? " + strings.Repeat("a", length) + "\n  : image: a\n    volumes: [/a:/b")
	for b.Len()+len(",/a:/b]\n") <= stack.MaxFile {
		b.WriteString(",/a:/b")
	}

	b.WriteString("]\n")

	return b.String()
}

// costliest is a file as large as a stack's may be, of as many services as it
// may have, each of whose commands is as many one-letter words as fit,
// written as a list, each a node YAML reads.
func costliest() string {
	var b strings.Builder

	b.WriteString("services:\n")

	room := (stack.MaxFile - b.Len()) / stack.MaxServices
	for i := range stack.MaxServices {
		start := b.Len()

		fmt.Fprintf(&b, "  s%02d:\n    image: a\n    command: [a", i)
		for b.Len()-start+len(",a]\n") <= room {
			b.WriteString(",a")
		}

		b.WriteString("]\n")
	}

	return b.String()
}

func TestParse_ports(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		port string
		code string
	}{
		"no port at all":                      {port: `"web"`, code: stack.CodeInvalidPortMapping},
		"a port of nothing":                   {port: `"0"`, code: stack.CodeInvalidPortMapping},
		"a port past the last":                {port: `"65536"`, code: stack.CodeInvalidPortMapping},
		"a host port past the last":           {port: `"70000:80"`, code: stack.CodeInvalidPortMapping},
		"a range the wrong way round":         {port: `"90-80"`, code: stack.CodeInvalidPortMapping},
		"ranges of different lengths":         {port: `"9000-9002:80-81"`, code: stack.CodeInvalidPortMapping},
		"a range for one port":                {port: `"9000-9002:80"`, code: stack.CodeInvalidPortMapping},
		"a protocol it does not know":         {port: `"80/sctp"`, code: "invalid_protocol"},
		"an address that is none":             {port: `"localhost:8080:80"`, code: "invalid_ip"},
		"an IPv6 address out of brackets":     {port: `"::1:8080:80"`, code: stack.CodeInvalidPortMapping},
		"an address in brackets and no host":  {port: `"[::1]:80"`, code: stack.CodeInvalidPortMapping},
		"too many parts":                      {port: `"1:2:3:4"`, code: stack.CodeInvalidPortMapping},
		"a range of too many ports":           {port: `"1-2000"`, code: "too_large"},
		"a list":                              {port: `[80]`, code: stack.CodeInvalidPortMapping},
		"a long one with no target":           {port: `{published: 8080}`, code: stack.CodeInvalidPortMapping},
		"a long one published on a range":     {port: `{target: 80, published: 8000-8001}`, code: stack.CodeInvalidPortMapping},
		"a long one on an address that is no": {port: `{target: 80, host_ip: nowhere}`, code: "invalid_ip"},
		"a long one of a protocol it lacks":   {port: `{target: 80, protocol: sctp}`, code: "invalid_protocol"},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			t.Parallel()

			_, invalid := stack.Parse("services:\n  web:\n    image: nginx\n    ports:\n      - " + tt.port + "\n")

			require.Len(t, invalid, 1, "%v", invalid)

			for path, code := range invalid {
				assert.True(t, strings.HasPrefix(path, "compose.services.web.ports[0]"), path)
				assert.Equal(t, tt.code, code)
			}
		})
	}
}

func TestParse_limits(t *testing.T) {
	t.Parallel()

	limited := func(key string, value string) string {
		return "services:\n  web:\n    image: nginx\n    resources:\n      limits:\n        " + key + ": " + value + "\n"
	}

	for name, tt := range map[string]struct {
		key   string
		value string
		code  string
	}{
		"cores of nothing":         {key: "cpus", value: "0", code: stack.CodeInvalidCPUs},
		"fewer cores than none":    {key: "cpus", value: "-1", code: stack.CodeInvalidCPUs},
		"cores that are no number": {key: "cpus", value: "half", code: stack.CodeInvalidCPUs},
		"cores of no end":          {key: "cpus", value: ".inf", code: stack.CodeInvalidCPUs},
		"cores as a list":          {key: "cpus", value: "[1]", code: stack.CodeInvalidCPUs},
		"memory of nothing":        {key: "memory", value: "0", code: stack.CodeInvalidMemory},
		"memory of no unit":        {key: "memory", value: "512x", code: stack.CodeInvalidMemory},
		"a fraction of a byte":     {key: "memory", value: "1.5", code: stack.CodeInvalidMemory},
		"less memory than none":    {key: "memory", value: "-512m", code: stack.CodeInvalidMemory},
		"memory in words":          {key: "memory", value: "lots", code: stack.CodeInvalidMemory},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			t.Parallel()

			_, invalid := stack.Parse(limited(tt.key, tt.value))

			assert.Equal(t, domain.ValidationErrors{"compose.services.web.resources.limits." + tt.key: tt.code}, invalid)
		})
	}

	for written, bytes := range map[string]uint64{
		"1048576": 1 << 20,
		"512k":    512 << 10,
		"512m":    512 << 20,
		"512M":    512 << 20,
		"2g":      2 << 30,
		"1.5g":    3 << 29,
		"64mb":    64 << 20,
		"100b":    100,
	} {
		t.Run("reads memory of "+written, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parse(limited("memory", written))
			require.Empty(t, invalid)

			assert.Equal(t, bytes, file.Services["web"].Memory)
		})
	}
}

func TestParse_mounts(t *testing.T) {
	t.Parallel()

	mounted := func(volume string) string {
		return "services:\n  web:\n    image: nginx\n    volumes:\n      - " + volume + "\nvolumes:\n  data:\n"
	}

	for name, tt := range map[string]struct {
		volume string
		code   string
	}{
		"a bind of a path relative to the file":   {volume: "./site:/srv", code: stack.CodeRelativeBind},
		"one above it":                            {volume: "../site:/srv", code: stack.CodeRelativeBind},
		"one of somebody's home":                  {volume: "~/site:/srv", code: stack.CodeRelativeBind},
		"one with no slash at its start":          {volume: "site/html:/srv", code: stack.CodeRelativeBind},
		"a long bind of a relative path":          {volume: "{type: bind, source: site, target: /srv}", code: stack.CodeRelativeBind},
		"a long bind of nothing":                  {volume: "{type: bind, target: /srv}", code: stack.CodeRelativeBind},
		"a volume with no name":                   {volume: "/data", code: stack.CodeAnonymousVolume},
		"a long volume with no name":              {volume: "{type: volume, target: /data}", code: stack.CodeAnonymousVolume},
		"a tmpfs":                                 {volume: "{type: tmpfs, target: /tmp}", code: stack.CodeInvalidVolumeType},
		"a long one of no type":                   {volume: "{source: data, target: /data}", code: stack.CodeInvalidVolumeType},
		"a target relative to anything":           {volume: "data:data", code: "invalid_mount"},
		"a mode it does not know":                 {volume: "data:/data:z", code: "invalid_mount"},
		"too many parts":                          {volume: "data:/data:ro:z", code: "invalid_mount"},
		"a long one with no target":               {volume: "{type: volume, source: data}", code: "invalid_mount"},
		"a long one read only in words":           {volume: "{type: volume, source: data, target: /data, read_only: yes}", code: "invalid_mount"},
		"a volume named as docker names nothing":  {volume: "da$ta:/data", code: "invalid_name"},
		"a mount that is a list":                  {volume: "[data, /data]", code: "invalid_mount"},
		"a mount that is nothing":                 {volume: "~", code: "invalid_mount"},
		"a bind's target that is not absolute":    {volume: "/srv:srv", code: "invalid_mount"},
		"a long bind's target that is not either": {volume: "{type: bind, source: /srv, target: srv}", code: "invalid_mount"},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			t.Parallel()

			_, invalid := stack.Parse(mounted(tt.volume))

			assert.Equal(t, domain.ValidationErrors{"compose.services.web.volumes[0]": tt.code}, invalid)
		})
	}

	t.Run("takes a bind of an absolute path and a declared volume, read only or not", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse("services:\n  web:\n    image: nginx\n    volumes:\n      - /srv:/srv\n      - data:/data:ro\n      - {type: bind, source: /etc/site, target: /etc/site, read_only: false}\nvolumes:\n  data:\n")
		require.Empty(t, invalid)

		assert.Equal(t, []stack.Mount{
			{Type: stack.MountBind, Source: "/srv", Target: "/srv"},
			{Type: stack.MountVolume, Source: "data", Target: "/data", ReadOnly: true},
			{Type: stack.MountBind, Source: "/etc/site", Target: "/etc/site"},
		}, file.Services["web"].Volumes)
	})
}

func TestParse_names(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		file string
		path string
	}{
		"a service":                 {file: "services:\n  'my web':\n    image: nginx\n", path: "compose.services.my web"},
		"one starting with a dash":  {file: "services:\n  -web:\n    image: nginx\n", path: "compose.services.-web"},
		"a volume":                  {file: "services:\n  web:\n    image: nginx\nvolumes:\n  _data:\n", path: "compose.volumes._data"},
		"a network":                 {file: "services:\n  web:\n    image: nginx\nnetworks:\n  front/end:\n", path: "compose.networks.front/end"},
		"a network a service joins": {file: "services:\n  web:\n    image: nginx\n    networks: [front!]\n", path: "compose.services.web.networks[0]"},
	} {
		t.Run("refuses "+name+" docker would not name", func(t *testing.T) {
			t.Parallel()

			_, invalid := stack.Parse(tt.file)

			assert.Equal(t, domain.ValidationErrors{tt.path: "invalid_name"}, invalid)
		})
	}

	t.Run("takes names of letters, digits, dots, dashes and underscores, from a letter or a digit", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse("services:\n  a:\n    image: nginx\n    networks: [front.end_1]\n  2-web_v1.0:\n    image: nginx\nnetworks:\n  front.end_1:\n")
		require.Empty(t, invalid)

		assert.Equal(t, []string{"2-web_v1.0", "a"}, file.ServiceNames())
	})

	longest := strings.Repeat("a", stack.MaxNameLength)
	long := longest + "a"

	t.Run("takes names as long as a name may be, and what names them", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse(strings.NewReplacer("NAME", longest).Replace(
			"services:\n  NAME:\n    image: nginx\n    networks: [NAME]\n    volumes: [NAME:/data, {type: volume, source: NAME, target: /backup}]\n" +
				"  web:\n    image: nginx\n    depends_on: [NAME]\n    networks: {NAME: }\n  api:\n    image: api\n    depends_on: {NAME: {condition: service_started}}\n" +
				"networks:\n  NAME:\nvolumes:\n  NAME:\n",
		))
		require.Empty(t, invalid)

		assert.Equal(t, []string{longest}, file.Services["web"].DependsOn)
		assert.Equal(t, []string{longest}, file.Networks)
		assert.Equal(t, []string{longest}, file.Volumes)
	})

	for name, tt := range map[string]struct {
		file string
		path string
	}{
		"a service":                                        {file: "services:\n  " + long + ":\n    image: nginx\n    build: .\n", path: "compose.services." + long},
		"a volume":                                         {file: "services:\n  web:\n    image: nginx\nvolumes:\n  " + long + ":\n    driver: local\n", path: "compose.volumes." + long},
		"a network":                                        {file: "services:\n  web:\n    image: nginx\nnetworks:\n  " + long + ":\n    driver: bridge\n", path: "compose.networks." + long},
		"a service it depends on":                          {file: "services:\n  web:\n    image: nginx\n    depends_on: [" + long + "]\n", path: "compose.services.web.depends_on[0]"},
		"a service it depends on in a mapping":             {file: "services:\n  web:\n    image: nginx\n    depends_on:\n      " + long + ":\n        condition: service_healthy\n", path: "compose.services.web.depends_on." + long},
		"a network it joins":                               {file: "services:\n  web:\n    image: nginx\n    networks: [" + long + "]\n", path: "compose.services.web.networks[0]"},
		"a network it joins in a mapping":                  {file: "services:\n  web:\n    image: nginx\n    networks:\n      " + long + ":\n        aliases: [www]\n", path: "compose.services.web.networks." + long},
		"a volume it mounts":                               {file: "services:\n  web:\n    image: nginx\n    volumes: [" + long + ":/data]\n", path: "compose.services.web.volumes[0]"},
		"a volume it mounts in its long form":              {file: "services:\n  web:\n    image: nginx\n    volumes: [{type: volume, source: " + long + ", target: /data}]\n", path: "compose.services.web.volumes[0]"},
		"a service of a name docker would not take either": {file: "services:\n  -" + longest + ":\n    image: nginx\n", path: "compose.services.-" + longest},
	} {
		t.Run("refuses "+name+" named longer than a name may be, once, and reads nothing beneath it", func(t *testing.T) {
			t.Parallel()

			_, invalid := stack.Parse(tt.file)

			assert.Equal(t, domain.ValidationErrors{tt.path: "too_large"}, invalid)
		})
	}
}

func TestFile_Levels(t *testing.T) {
	t.Parallel()

	t.Run("a chain is a level a service", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse("services:\n  web:\n    image: nginx\n    depends_on: [api]\n  api:\n    image: api\n    depends_on: [db]\n  db:\n    image: postgres\n")
		require.Empty(t, invalid)

		assert.Equal(t, [][]string{{"db"}, {"api"}, {"web"}}, file.Levels())
	})

	t.Run("what depends on nothing shares the first level, in the order of its names", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse("services:\n  web:\n    image: nginx\n    depends_on: [db, cache]\n  db:\n    image: postgres\n  cache:\n    image: redis\n  worker:\n    image: worker\n")
		require.Empty(t, invalid)

		assert.Equal(t, [][]string{{"cache", "db", "worker"}, {"web"}}, file.Levels())
	})

	t.Run("services in a ring are in none, which a stack's plan refuses", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parse("services:\n  web:\n    image: nginx\n    depends_on: [api]\n  api:\n    image: api\n    depends_on: [web]\n  db:\n    image: postgres\n")
		require.Empty(t, invalid, "reading a file says nothing of rings")

		assert.Equal(t, [][]string{{"db"}}, file.Levels())
	})
}

func TestWords(t *testing.T) {
	t.Parallel()

	for line, words := range map[string][]string{
		"":                                {},
		"   ":                             {},
		"nginx -g 'daemon off;'":          {"nginx", "-g", "daemon off;"},
		`echo "a \"quoted\" word"`:        {"echo", `a "quoted" word`},
		`echo "a \n b"`:                   {"echo", `a \n b`},
		`echo "\$HOME" \$HOME $HOME`:      {"echo", "$HOME", "$HOME", "$HOME"},
		`echo a\ b`:                       {"echo", "a b"},
		"echo ''":                         {"echo", ""},
		`echo ""x`:                        {"echo", "x"},
		"sh -c 'echo $1' --\tone\ntwo":    {"sh", "-c", "echo $1", "--", "one", "two"},
		"echo 'it''s'":                    {"echo", "its"},
		`printf "%s\\n" done`:             {"printf", `%s\n`, "done"},
		"line \\\ncontinued":              {"line", "continued"},
		`grep -E "^[a-z]+$" 'file name'`:  {"grep", "-E", "^[a-z]+$", "file name"},
		`one"two"'three'four`:             {"onetwothreefour"},
		"`backquoted` stays as it is":     {"`backquoted`", "stays", "as", "it", "is"},
		`echo "\` + "`" + `"`:             {"echo", "`"},
		"echo * ~ ?":                      {"echo", "*", "~", "?"},
		"a  \t  b":                        {"a", "b"},
		"tail -f /var/log/nginx/acc.log ": {"tail", "-f", "/var/log/nginx/acc.log"},
	} {
		t.Run("splits "+line, func(t *testing.T) {
			t.Parallel()

			split, ok := stack.Words(line)
			require.True(t, ok)

			if len(words) == 0 {
				assert.Empty(t, split)
			} else {
				assert.Equal(t, words, split)
			}
		})
	}

	for _, line := range []string{"echo 'open", `echo "open`, `echo "open\"`, `trailing\`} {
		t.Run("does not split "+line, func(t *testing.T) {
			t.Parallel()

			split, ok := stack.Words(line)

			assert.False(t, ok)
			assert.Nil(t, split)
		})
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	for status, tt := range map[string]struct {
		code int
		says bool
	}{
		"Exited (0) 3 seconds ago":    {code: 0, says: true},
		"Exited (137) 1 minute ago":   {code: 137, says: true},
		"Exited (-1) a while ago":     {code: -1, says: true},
		"Up 3 seconds":                {},
		"Created":                     {},
		"Exited 3 seconds ago":        {},
		"exited (0) 3 seconds ago":    {},
		"Restarting (1) 1 second ago": {},
	} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()

			code, says := stack.ExitCode(status)

			assert.Equal(t, tt.code, code)
			assert.Equal(t, tt.says, says)
		})
	}
}
