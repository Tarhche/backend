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

		file, invalid := stack.Parsed(t, everything)
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

		file, invalid := stack.Parsed(t, `
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

	t.Run("a variable is named as it says, a dollar or a dot among it, and is a word of its service's environment, as the rest are", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    environment:\n      $date: x\n      $numberLong: '5'\n      a.b: c\n")
		require.Empty(t, invalid)
		assert.Equal(t, []string{"$date=x", "$numberLong=5", "a.b=c"}, file.Services["web"].Environment)
	})

	t.Run("a service that names no network joins the default one, which is made only then", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    networks: [front]\nnetworks:\n  front:\n  back:\n")
		require.Empty(t, invalid)
		assert.Equal(t, []string{"back", "front"}, file.NetworksMade(), "every one it declares, and no default one nothing joins")

		file, invalid = stack.Parsed(t, "services:\n  web:\n    image: nginx\n  db:\n    image: postgres\n    networks: {default: }\n")
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

			file, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}

	t.Run("refuses a file larger than a stack may be given", func(t *testing.T) {
		t.Parallel()

		_, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n"+strings.Repeat("#", stack.MaxFile))

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

			_, invalid := stack.Parsed(t, tt.file)

			require.Contains(t, invalid, tt.path, "%v", invalid)

			if strings.HasSuffix(name, "no mapping") || strings.HasSuffix(name, "as anything") {
				assert.Equal(t, "invalid_value", invalid[tt.path])
			} else {
				assert.Equal(t, stack.CodeUnsupportedKey, invalid[tt.path])
			}
		})
	}
}

func TestParse_keys(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"a key at the top it does not take":                           {file: "services:\n  web: {image: a}\nbogus: 1\n", invalid: domain.ValidationErrors{"compose.bogus": stack.CodeUnsupportedKey}},
		"one of a service":                                            {file: "services:\n  web: {image: a, bogus: 1}\n", invalid: domain.ValidationErrors{"compose.services.web.bogus": stack.CodeUnsupportedKey}},
		"a healthcheck, whole":                                        {file: "services:\n  web: {image: a, healthcheck: {test: x}}\n", invalid: domain.ValidationErrors{"compose.services.web.healthcheck": stack.CodeUnsupportedKey}},
		"one of its resources":                                        {file: "services:\n  web: {image: a, resources: {limits: {cpus: 1}, bogus: 1}}\n", invalid: domain.ValidationErrors{"compose.services.web.resources.bogus": stack.CodeUnsupportedKey}},
		"one of its limits":                                           {file: "services:\n  web: {image: a, resources: {limits: {cpus: 1, bogus: 1}}}\n", invalid: domain.ValidationErrors{"compose.services.web.resources.limits.bogus": stack.CodeUnsupportedKey}},
		"a limit of cores that is a mapping":                          {file: "services:\n  web: {image: a, resources: {limits: {cpus: {bogus: 1}}}}\n", invalid: domain.ValidationErrors{"compose.services.web.resources.limits.cpus": stack.CodeInvalidCPUs}},
		"one of a port in its long form":                              {file: "services:\n  web: {image: a, ports: [{target: 80, bogus: 1}]}\n", invalid: domain.ValidationErrors{"compose.services.web.ports[0].bogus": stack.CodeUnsupportedKey}},
		"one of a volume in its long form":                            {file: "services:\n  web: {image: a, volumes: [{type: bind, source: /a, target: /b, bogus: 1}]}\n", invalid: domain.ValidationErrors{"compose.services.web.volumes[0].bogus": stack.CodeUnsupportedKey}},
		"a bind's options":                                            {file: "services:\n  web: {image: a, volumes: [{type: bind, source: /a, target: /b, bind: {propagation: shared}}]}\n", invalid: domain.ValidationErrors{"compose.services.web.volumes[0].bind": stack.CodeUnsupportedKey}},
		"one of what a service depended on is depended on with":       {file: "services:\n  db: {image: a}\n  web: {image: a, depends_on: {db: {condition: service_started, bogus: 1}}}\n", invalid: domain.ValidationErrors{"compose.services.web.depends_on.db.bogus": stack.CodeUnsupportedKey}},
		"a dependency's restart":                                      {file: "services:\n  db: {image: a}\n  web: {image: a, depends_on: {db: {condition: service_started, restart: true}}}\n", invalid: domain.ValidationErrors{"compose.services.web.depends_on.db.restart": stack.CodeUnsupportedKey}},
		"what a service says of a network it joins":                   {file: "networks: {n: ~}\nservices:\n  web: {image: a, networks: {n: {aliases: [x]}}}\n", invalid: domain.ValidationErrors{"compose.services.web.networks.n.aliases": stack.CodeUnsupportedKey}},
		"what the file says of a network it declares":                 {file: "networks: {n: {driver: overlay}}\nservices:\n  web: {image: a}\n", invalid: domain.ValidationErrors{"compose.networks.n.driver": stack.CodeUnsupportedKey}},
		"and of a volume":                                             {file: "volumes: {v: {external: true}}\nservices:\n  web: {image: a}\n", invalid: domain.ValidationErrors{"compose.volumes.v.external": stack.CodeUnsupportedKey}},
		"a service's labels, under whatever keys":                     {file: "services:\n  web: {image: a, labels: {$date: x}}\n", invalid: domain.ValidationErrors{"compose.services.web.labels": stack.CodeUnsupportedKey}},
		"a network's":                                                 {file: "networks: {n: {labels: {$numberLong: '5'}}}\nservices:\n  web: {image: a}\n", invalid: domain.ValidationErrors{"compose.networks.n.labels": stack.CodeUnsupportedKey}},
		"and a volume's":                                              {file: "volumes: {v: {labels: {a.b: c}}}\nservices:\n  web: {image: a}\n", invalid: domain.ValidationErrors{"compose.volumes.v.labels": stack.CodeUnsupportedKey}},
		"a variable that is a mapping":                                {file: "services:\n  web: {image: a, environment: {A: {b: 1}}}\n", invalid: domain.ValidationErrors{"compose.services.web.environment.A": "invalid_value"}},
		"a word of a command that is a mapping":                       {file: "services:\n  web: {image: a, command: [{b: 1}]}\n", invalid: domain.ValidationErrors{"compose.services.web.command[0]": "invalid_value"}},
		"a merge of a mapping":                                        {file: "services:\n  web: {image: a, <<: {user: root}}\n", invalid: domain.ValidationErrors{"compose.services.web.<<": stack.CodeUnsupportedKey}},
		"one tagged a merge":                                          {file: "services:\n  web: {image: a, !!merge <<: {user: root}}\n", invalid: domain.ValidationErrors{"compose.services.web.<<": stack.CodeUnsupportedTag}},
		"and << tagged a string, which is a key it does not take":     {file: "services:\n  web: {image: a, !!str <<: {user: root}}\n", invalid: domain.ValidationErrors{"compose.services.web.<<": stack.CodeUnsupportedKey}},
		"a key anchored":                                              {file: "services:\n  &k web: {image: a}\n", invalid: domain.ValidationErrors{"compose.services.web": stack.CodeUnsupportedAlias}},
		"a value anchored in a flow mapping":                          {file: "services: {web: {image: &i a}}\n", invalid: domain.ValidationErrors{"compose.services.web.image": stack.CodeUnsupportedAlias}},
		"a merge of an alias, said as an alias before as a merge":     {file: "x: &b {image: a}\nservices:\n  web: {<<: *b}\n", invalid: domain.ValidationErrors{"compose.x": stack.CodeUnsupportedAlias, "compose.services.web.<<": stack.CodeUnsupportedAlias}},
		"a second document, with an alias in it":                      {file: "services:\n  web: {image: a}\n---\nx: &a 1\ny: *a\n", invalid: domain.ValidationErrors{"compose": "invalid_value"}},
		"a second document that is empty":                             {file: "services:\n  web: {image: a}\n---\n", invalid: domain.ValidationErrors{"compose": "invalid_value"}},
		"a key a tag of its own says is anything":                     {file: "services:\n  web: {!custom image: a}\n", invalid: domain.ValidationErrors{"compose.services.web.image": stack.CodeUnsupportedTag}},
		"a key binary, whose text is a key it takes":                  {file: "services:\n  web: {image: a, !!binary user: root}\n", invalid: domain.ValidationErrors{"compose.services.web.user": stack.CodeUnsupportedTag}},
		"and so of its limits":                                        {file: "services:\n  web: {image: a, resources: {limits: {!!binary cpus: 2}}}\n", invalid: domain.ValidationErrors{"compose.services.web.resources.limits.cpus": stack.CodeUnsupportedTag}},
		"the networks a service joins":                                {file: "networks: {n: ~}\nservices:\n  web: {image: a, !!binary networks: [n]}\n", invalid: domain.ValidationErrors{"compose.services.web.networks": stack.CodeUnsupportedTag}},
		"the networks the file declares":                              {file: "services:\n  web: {image: a}\n!!binary networks: {front: ~}\n", invalid: domain.ValidationErrors{"compose.networks": stack.CodeUnsupportedTag}},
		"and its services":                                            {file: "!!binary services:\n  web: {image: a}\n", invalid: domain.ValidationErrors{"compose.services": stack.CodeUnsupportedTag}},
		"a volume's name binary, whose text is read as ../":           {file: "services:\n  web: {image: a}\nvolumes:\n  !!binary Li4v: ~\n", invalid: domain.ValidationErrors{"compose.volumes.Li4v": stack.CodeUnsupportedTag}},
		"and a network's":                                             {file: "services:\n  web: {image: a}\nnetworks:\n  !!binary Li4v: ~\n", invalid: domain.ValidationErrors{"compose.networks.Li4v": stack.CodeUnsupportedTag}},
		"and a service's, whose text is read as another's name":       {file: "services:\n  web: {image: a, privileged: false}\n  !!binary d2Vi: {image: a, privileged: true}\n", invalid: domain.ValidationErrors{"compose.services.d2Vi": stack.CodeUnsupportedTag}},
		"a volume's name that is nothing":                             {file: "services:\n  web: {image: a}\nvolumes:\n  null: ~\n", invalid: domain.ValidationErrors{"compose.volumes.null": stack.CodeInvalidKey}},
		"a key that is a list, under the path of its mapping's entry": {file: "services:\n  web: {image: a, ? [x, y] : 1}\n", invalid: domain.ValidationErrors{"compose.services.web.": stack.CodeInvalidKey}},
		"a key given twice":                                           {file: "services:\n  web: {image: a, image: b}\n", invalid: domain.ValidationErrors{"compose.services.web.image": stack.CodeDuplicateKey}},
		"resources tagged as nothing, before a key of its limits":     {file: "services:\n  web: {image: a, resources: !!null {limits: {bogus: 1}}}\n", invalid: domain.ValidationErrors{"compose.services.web.resources": stack.CodeUnsupportedTag}},
		"a volume's name that is a number":                            {file: "services:\n  web: {image: a}\nvolumes:\n  1: ~\n", invalid: domain.ValidationErrors{"compose.volumes.1": stack.CodeInvalidKey}},
		"a network's that is a boolean":                               {file: "services:\n  web: {image: a}\nnetworks:\n  true: ~\n", invalid: domain.ValidationErrors{"compose.networks.true": stack.CodeInvalidKey}},
		"a service's that is a date":                                  {file: "services:\n  2026-10-11: {image: a}\n", invalid: domain.ValidationErrors{"compose.services.2026-10-11": stack.CodeInvalidKey}},
		"a variable's that is a number":                               {file: "services:\n  web: {image: a, environment: {1: a}}\n", invalid: domain.ValidationErrors{"compose.services.web.environment.1": stack.CodeInvalidKey}},
		"a service depended on that is a number, in a mapping":        {file: "services:\n  web: {image: a, depends_on: {1: ~}}\n", invalid: domain.ValidationErrors{"compose.services.web.depends_on.1": stack.CodeInvalidKey}},
	} {
		t.Run("refuses "+name+", saying where", func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}

	t.Run("takes a name YAML would read as anything but a string quoted, and reads it, and what names it, as it is written", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  '1':\n    image: a\n    volumes: ['2026-10-11:/data']\n    networks: ['true']\n  web:\n    image: a\n    depends_on: [1]\n    environment: {'2': b}\nvolumes:\n  '2026-10-11':\nnetworks:\n  \"true\":\n")
		require.Empty(t, invalid)

		assert.Equal(t, []string{"1", "web"}, file.ServiceNames())
		assert.Equal(t, []string{"2026-10-11"}, file.Volumes)
		assert.Equal(t, []string{"true"}, file.Networks)
		assert.Equal(t, []string{"true"}, file.Services["1"].Networks)
		assert.Equal(t, []string{"1"}, file.Services["web"].DependsOn)
		assert.Equal(t, []string{"2=b"}, file.Services["web"].Environment)
	})

	t.Run("takes a file that says where its document ends", func(t *testing.T) {
		t.Parallel()

		parsed, invalid := stack.Parsed(t, "services:\n  web: {image: a}\n...\n")
		require.Empty(t, invalid)

		assert.Equal(t, "a", parsed.Services["web"].Image)
	})
}

func TestParse_tags(t *testing.T) {
	t.Parallel()

	// service is a file of the one service web, which says what more says
	// beside its image, a.
	service := func(more string) string {
		return "services:\n  web: {image: a" + more + "}\n"
	}

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"a value a tag of its own says is anything": {
			file:    "services:\n  web: {image: !custom a}\n",
			invalid: domain.ValidationErrors{"compose.services.web.image": stack.CodeUnsupportedTag},
		},
		"a value binary, whose text is not what it says": {
			file:    "services:\n  web: {image: !!binary bmdpbng=}\n",
			invalid: domain.ValidationErrors{"compose.services.web.image": stack.CodeUnsupportedTag},
		},
		"a value tagged as nothing": {
			file:    service(", user: !!null ~"),
			invalid: domain.ValidationErrors{"compose.services.web.user": stack.CodeUnsupportedTag},
		},
		"a service's mapping tagged as nothing": {
			file:    "services:\n  web: !!null {image: a}\n",
			invalid: domain.ValidationErrors{"compose.services.web": stack.CodeUnsupportedTag},
		},
		"a mapping tagged as what it is": {
			file:    "services:\n  web: !!map {image: a}\n",
			invalid: domain.ValidationErrors{"compose.services.web": stack.CodeUnsupportedTag},
		},
		"a list tagged as what it is": {
			file:    service(", command: !!seq [run]"),
			invalid: domain.ValidationErrors{"compose.services.web.command": stack.CodeUnsupportedTag},
		},
		"an item of a list tagged as a number": {
			file:    service(", command: [run, !!int 1]"),
			invalid: domain.ValidationErrors{"compose.services.web.command[1]": stack.CodeUnsupportedTag},
		},
		"the file itself tagged": {
			file:    "--- !!map\nservices:\n  web: {image: a}\n",
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedTag},
		},
		"resources tagged as nothing, with limits beneath them": {
			file:    service(", resources: !!null {limits: {cpus: 1}}"),
			invalid: domain.ValidationErrors{"compose.services.web.resources": stack.CodeUnsupportedTag},
		},
		"and with nothing beneath them": {
			file:    service(", resources: !!null {}"),
			invalid: domain.ValidationErrors{"compose.services.web.resources": stack.CodeUnsupportedTag},
		},
		"and over text": {
			file:    service(", resources: !!null abc"),
			invalid: domain.ValidationErrors{"compose.services.web.resources": stack.CodeUnsupportedTag},
		},
		"limits tagged as nothing, with a limit beneath them": {
			file:    service(", resources: {limits: !!null {cpus: 1}}"),
			invalid: domain.ValidationErrors{"compose.services.web.resources.limits": stack.CodeUnsupportedTag},
		},
		"and over text, as limits": {
			file:    service(", resources: {limits: !!null abc}"),
			invalid: domain.ValidationErrors{"compose.services.web.resources.limits": stack.CodeUnsupportedTag},
		},
	} {
		t.Run("refuses "+name+", saying where", func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}

	t.Run("takes a key and a value tagged as strings, which they are, as what is written, and a tag that says nothing", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  web: {image: !!str a, user: ! root, environment: {!!str 1: !!str 2}}\n")
		require.Empty(t, invalid)

		web := file.Services["web"]
		assert.Equal(t, "a", web.Image)
		assert.Equal(t, "root", web.User)
		assert.Equal(t, []string{"1=2"}, web.Environment)
	})
}

func TestParse_directives(t *testing.T) {
	t.Parallel()

	const web = "services:\n  web:\n    image: a\n"

	// continued is a file of the one service web, whose command is a quoted
	// value carried on over as many lines that begin with % as lines says,
	// each after a break of its own, YAML's every kind of them in turn, and
	// the words it is: a line separator and a paragraph separator end a line
	// as the others do, but are kept in the value, as YAML 1.1 has it, and
	// so join the words they are between.
	continued := func(lines int) (string, []string) {
		breaks := []string{"\n", "\r", "\r\n", "\u0085", "\u2028", "\u2029"}

		var b strings.Builder

		words := []string{"printf"}

		b.WriteString(web + "    command: \"printf")
		for i := range lines {
			kept := breaks[i%len(breaks)]
			b.WriteString(kept + "%s")

			if kept == "\u2028" || kept == "\u2029" {
				words[len(words)-1] += kept + "%s"
			} else {
				words = append(words, "%s")
			}
		}

		b.WriteString("\"\n")

		return b.String(), words
	}

	most, mostWords := continued(stack.MaxPercentLines)
	more, _ := continued(stack.MaxPercentLines + 1)

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"a %TAG directive, which would have a tag read as a string": {
			file:    "%TAG !s! tag:yaml.org,2002:\n---\n" + web + "    user: !s!str a\n",
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective},
		},
		"a %YAML directive": {
			file:    "%YAML 1.1\n---\n" + web,
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective},
		},
		"one YAML does not know": {
			file:    "%FOO bar\n---\n" + web,
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective},
		},
		"one after blank lines and comments": {
			file:    "# a stack\n\n  # of one service\n%TAG !a! b\n---\n" + web,
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective},
		},
		"one after the mark that says the file is UTF-8": {
			file:    "\ufeff%TAG !a! b\n---\n" + web,
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective},
		},
		"one after a line ended by a carriage return": {
			file:    "# a stack\r%TAG !a! b\r---\r" + web,
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective},
		},
		"more lines that begin with % than a file may have, each carrying a quoted value on": {
			file:    more,
			invalid: domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective},
		},
		"a directive after the document, which begins one of its own": {
			file:    web + "%TAG !a! b\n---\nx: 1\n",
			invalid: domain.ValidationErrors{"compose": "invalid_value"},
		},
		"and one after the document's end": {
			file:    web + "...\n%TAG !a! b\n---\nx: 1\n",
			invalid: domain.ValidationErrors{"compose": "invalid_value"},
		},
		"a file in UTF-16, which YAML would read by the mark it begins with": {
			file: func() string {
				b := []byte{0xFF, 0xFE}
				for _, c := range "%TAG !a! b\n---\n" + web {
					b = append(b, byte(c), 0)
				}

				return string(b)
			}(),
			invalid: domain.ValidationErrors{"compose": "invalid_value"},
		},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}

	for name, tt := range map[string]struct {
		file  string
		words []string
	}{
		"a double-quoted value carried on to a line that begins with %": {
			file:  web + "    command: \"printf\n%s hi\"\n",
			words: []string{"printf", "%s", "hi"},
		},
		"and over an empty line": {
			file:  web + "    command: \"printf\n\n%s\"\n",
			words: []string{"printf", "%s"},
		},
		"a single-quoted one": {
			file:  web + "    command: 'printf\n%s hi'\n",
			words: []string{"printf", "%s", "hi"},
		},
		"a plain one in brackets": {
			file:  web + "    command: [printf\n%s]\n",
			words: []string{"printf %s"},
		},
		"each in a file that is a mapping in braces": {
			file:  "{services: {web: {image: a, command: [\"printf\n%s\", x\n%y]}}}\n",
			words: []string{"printf %s", "x %y"},
		},
		"as many lines that begin with % as a file may have, after each of YAML's breaks": {
			file:  most,
			words: mostWords,
		},
	} {
		t.Run("takes "+name, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parsed(t, tt.file)
			require.Empty(t, invalid)

			assert.Equal(t, tt.words, file.Services["web"].Command)
		})
	}
}

func TestParse_keyLength(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"a key as long as a key may be is read, and refused as a key it does not take": {
			file:    "services:\n  web: {image: a}\n? " + strings.Repeat("k", stack.MaxKeyLength) + "\n: 1\n",
			invalid: domain.ValidationErrors{"compose." + strings.Repeat("k", stack.MaxKeyLength): stack.CodeUnsupportedKey},
		},
		"and so is one as long in letters each more than a byte, counted in letters": {
			file:    "services:\n  web: {image: a}\n? " + strings.Repeat("ک", stack.MaxKeyLength) + "\n: 1\n",
			invalid: domain.ValidationErrors{"compose." + strings.Repeat("ک", stack.MaxKeyLength): stack.CodeUnsupportedKey},
		},
		"one a letter longer is refused as too large, once, under its own path, and nothing beneath it is read, an anchor among it": {
			file:    "services:\n  web: {image: a}\n? " + strings.Repeat("k", stack.MaxKeyLength+1) + "\n: {a: &x b, c: *x}\n",
			invalid: domain.ValidationErrors{"compose." + strings.Repeat("k", stack.MaxKeyLength+1): "too_large"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
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

			file, invalid := stack.Parsed(t, tt.file)

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

			file, invalid := stack.Parsed(t, tt.file)

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

			file, invalid := stack.Parsed(t, tt.file)

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

		file, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    ports: [\"1-1000:1-1000\"]\n")
		require.Empty(t, invalid)

		ports := file.Services["web"].Ports
		require.Len(t, ports, stack.MaxPorts)
		assert.Equal(t, stack.Port{Container: 1000, Host: 1000, Protocol: "tcp"}, ports[stack.MaxPorts-1])

		_, invalid = stack.Parsed(t, "services:\n  web:\n    image: nginx\n    ports: [\"1-65535\"]\n")
		assert.Equal(t, domain.ValidationErrors{"compose.services.web.ports[0]": "too_large"}, invalid)
	})

	// mounting is web joining the network n and mounting the volume v as
	// many times as it is said to, at as many targets, each a reference, the
	// network's one more; and then db, after it, which depends on a service
	// the file does not have, which no reference after the most a file makes
	// is read far enough to say.
	mounting := func(mounts int, network string) string {
		var b strings.Builder

		b.WriteString("services:\n  web:\n    image: nginx\n" + network + "    volumes:\n")
		for i := range mounts {
			fmt.Fprintf(&b, "      - v:/m%d\n", i)
		}

		b.WriteString("  db:\n    image: postgres\n    depends_on: [nope]\nnetworks:\n  n:\nvolumes:\n  v:\n")

		return b.String()
	}

	t.Run("its services make as many references as a file may, each network a service joins, each volume it mounts by its name and each service it depends on", func(t *testing.T) {
		t.Parallel()

		// web's network and its mounts, db's dependency on it and the default
		// network db joins.
		file, invalid := stack.Parsed(t, strings.Replace(mounting(stack.MaxReferences-3, "    networks: [n]\n"), "depends_on: [nope]", "depends_on: [web]", 1))
		require.Empty(t, invalid)

		assert.Len(t, file.Services["web"].Volumes, stack.MaxReferences-3)
		assert.Equal(t, []string{"web"}, file.Services["db"].DependsOn)
		assert.Equal(t, []string{stack.DefaultNetwork}, file.Services["db"].Networks, "the default one db joins, naming none, being the last of them")
	})

	for name, tt := range map[string]struct {
		file    string
		invalid domain.ValidationErrors
	}{
		"one more is refused where it is, and no reference after it is read": {
			file:    mounting(stack.MaxReferences, "    networks: [n]\n"),
			invalid: domain.ValidationErrors{"compose.services.web.volumes[999]": "too_large"},
		},
		"the default network a service that names none joins is one, under its networks": {
			file:    mounting(stack.MaxReferences, ""),
			invalid: domain.ValidationErrors{"compose.services.web.networks": "too_large"},
		},
		"and so is each service a service depends on, in the order the file says them": {
			file: func() string {
				var b strings.Builder

				b.WriteString("services:\n")
				for i := range stack.MaxServices {
					fmt.Fprintf(&b, "  s%d:\n    image: nginx\n    depends_on: [", i)
					for j := range i {
						fmt.Fprintf(&b, "s%d, ", j)
					}

					b.WriteString("nope]\n")
				}

				return b.String()
			}(),
			// s0 to s42 make 989, a dependency of each on each before it, on
			// nope, which is no service and is refused, and the default
			// network: s43's twelfth dependency is the 1001st, and its nope
			// is not read, nor anything after it.
			invalid: func() domain.ValidationErrors {
				invalid := domain.ValidationErrors{"compose.services.s43.depends_on[11]": "too_large"}
				for i := range 43 {
					invalid[fmt.Sprintf("compose.services.s%d.depends_on.nope", i)] = stack.CodeUndefinedService
				}

				return invalid
			}(),
		},
		"and each network of a mapping a service joins": {
			file: func() string {
				var b strings.Builder

				b.WriteString("networks:\n")
				for i := range stack.MaxNetworks {
					fmt.Fprintf(&b, "  n%d:\n", i)
				}

				b.WriteString("services:\n")
				for i := range stack.MaxReferences/stack.MaxNetworks + 1 {
					fmt.Fprintf(&b, "  s%d:\n    image: nginx\n    networks:\n", i)
					for j := range stack.MaxNetworks {
						fmt.Fprintf(&b, "      n%d:\n", j)
					}
				}

				return b.String()
			}(),
			invalid: domain.ValidationErrors{fmt.Sprintf("compose.services.s%d.networks.n0", stack.MaxReferences/stack.MaxNetworks): "too_large"},
		},
	} {
		t.Run("of references, "+name, func(t *testing.T) {
			t.Parallel()

			file, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, tt.invalid, invalid)
			assert.Empty(t, file.Services, "and is no file")
		})
	}
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

		_, invalid := stack.Parsed(t, b.String())
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

		_, invalid := stack.Parsed(t, b.String())
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

		_, invalid := stack.Parsed(t, b.String())
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
// whether it was refused or not, or up to ten, by the same path said again
// and again beneath a long key, is refused or read taking a few megabytes,
// or ten or so where as much is said of it as ever is; and a file as large
// as a stack's may be, as costly as the bounds let one be, is read in a few
// tens of them, read into nodes once to be read, and again to be decoded
// strictly (yaml.Decoder.KnownFields), which a file that is taken is. What
// YAML takes to read a file into nodes, before anything of the reading
// bounds it, is bounded by MaxFile alone: the costliest files a review
// found, comment lines in columns that alternate, each of which YAML keeps a
// record of its own of, and keys of one letter in braces, each two nodes of
// YAML's, are read in 48 MB at most, and refused in 24, which each took four
// times as much when a file could be 256 KiB. A file of as many %TAG
// directives as fit, which YAML reads each against every one before it,
// taking seconds, is refused before YAML reads any of it.
//
// It runs alone, not in parallel, since what other tests allocated meanwhile
// would be counted with what reading the files does.
func TestParse_bounded(t *testing.T) {
	const megabyte = 1 << 20

	// longest is a name as long as a name may be.
	longest := strings.Repeat("a", stack.MaxNameLength)

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
		"16 KiB of ranges of a thousand ports, no alias among them": {
			file: ranges(16 << 10),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services.web.ports[1]": "too_large"}, invalid)
			},
			most: 2 * megabyte,
		},
		"as many of them as a file may be": {
			file: ranges(stack.MaxFile),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services.web.ports[1]": "too_large"}, invalid)
			},
			most: 4 * megabyte,
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
			most: 4 * megabyte,
		},
		"as many %TAG directives as fit, each of a handle of its own, before a document that uses none of them, each of which YAML would have read against every one before it": {
			file: tagDirectives(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose": stack.CodeUnsupportedDirective}, invalid)
			},
			most: megabyte,
		},
		"a file as large as one may be, as costly to read as one can be: as many services as it may have, and each word of their commands a node of its own": {
			file: costliest(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 32 * megabyte,
		},
		"and one service whose command is as many words as fit, one list of them, which is costlier still": {
			file: func() string {
				var b strings.Builder

				b.WriteString("services:\n  web:\n    image: a\n    command: [a")
				for b.Len()+len(",a]\n") <= stack.MaxFile {
					b.WriteString(",a")
				}

				b.WriteString("]\n")

				return b.String()
			}(),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 32 * megabyte,
		},
		"a service named by 32 KiB, an explicit key's, longer than a key may be, which is refused once, and nothing beneath it read, though its command's last word is anchored": {
			file: anchoredBeneath(32<<10, 0),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + strings.Repeat("a", 32<<10): "too_large"}, invalid)
			},
			most: 8 * megabyte,
		},
		"a service named by as many characters as a key may have, its command as many words as fit, the last of them anchored, which is said under its path once": {
			file: anchoredBeneath(stack.MaxKeyLength, 0),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				require.Len(t, invalid, 1)

				for path, code := range invalid {
					assert.True(t, strings.HasPrefix(path, "compose.services."+strings.Repeat("a", stack.MaxKeyLength)+".command["), "%.40s...", path)
					assert.Equal(t, stack.CodeUnsupportedAlias, code)
				}
			},
			most: 16 * megabyte,
		},
		"and with a hundred and one anchored before the rest, which are said as many as are ever said, and nothing is read after them": {
			file: anchoredBeneath(stack.MaxKeyLength, stack.MaxInvalid+1),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Len(t, invalid, stack.MaxInvalid)

				for path, code := range invalid {
					assert.Equal(t, stack.CodeUnsupportedAlias, code, "%.40s...", path)
				}
			},
			most: 16 * megabyte,
		},
		"a key of 32 KiB whose mapping says an alias under one key as often as fit, which is refused once, as too large, and nothing beneath it read": {
			file: longKeyed("x: &x a\n", "1: *x"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.x": stack.CodeUnsupportedAlias, "compose." + strings.Repeat("a", 32<<10): "too_large"}, invalid)
			},
			most: 8 * megabyte,
		},
		"and one whose mapping says a key that is a number as often": {
			file: longKeyed("", "1: a"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose." + strings.Repeat("a", 32<<10): "too_large"}, invalid)
			},
			most: 8 * megabyte,
		},
		"and a key that is a list": {
			file: longKeyed("", "[a]: 1"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose." + strings.Repeat("a", 32<<10): "too_large"}, invalid)
			},
			most: 8 * megabyte,
		},
		"and a key that is nothing": {
			file: longKeyed("", "? : 1"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose." + strings.Repeat("a", 32<<10): "too_large"}, invalid)
			},
			most: 8 * megabyte,
		},
		"keys as long as a key may be, fourteen levels of them, whose innermost mapping says an alias under one key as often as fit, which is said as often as is ever said": {
			file: deepKeyed(14, "1: *x"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.x": stack.CodeUnsupportedAlias, keyedPath(14) + ".1": stack.CodeUnsupportedAlias}, invalid)
			},
			most: 16 * megabyte,
		},
		"and a key that is a number as often": {
			file: deepKeyed(14, "1: a"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.x": stack.CodeUnsupportedAlias, keyedPath(14) + ".1": stack.CodeInvalidKey}, invalid)
			},
			most: 16 * megabyte,
		},
		"and a key that is a list as often": {
			file: deepKeyed(14, "[a]: 1"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.x": stack.CodeUnsupportedAlias, keyedPath(14) + ".": stack.CodeInvalidKey}, invalid)
			},
			most: 16 * megabyte,
		},
		"and a key that is nothing as often": {
			file: deepKeyed(14, "? : 1"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.x": stack.CodeUnsupportedAlias, keyedPath(14) + ".": stack.CodeInvalidKey}, invalid)
			},
			most: 16 * megabyte,
		},
		"and fifteen levels of them, whose innermost mapping says one key as often as fit, each holding a list too deep to be read": {
			file: deepKeyed(15, "k: [a]"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.x": stack.CodeUnsupportedAlias, keyedPath(15) + ".k[0]": stack.CodeTooDeep}, invalid)
			},
			most: 16 * megabyte,
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
			most: 4 * megabyte,
		},
		"a service named by 32 KiB over as many binds as fit, whose name is refused once, and nothing of it read": {
			file: boundMounts(32 << 10),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + strings.Repeat("a", 32<<10): "too_large"}, invalid)
			},
			most: 4 * megabyte,
		},
		"a service named as long as a name may be over as many binds as fit, each path of which is written out only when it is refused": {
			file: boundMounts(stack.MaxNameLength),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 16 * megabyte,
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
			most: 16 * megabyte,
		},
		"a service whose command is a word and as many comment lines as fit, each in the column the one before it is not in, each of which YAML keeps a record of its own of, and of the line's after it": {
			file: filled("services:\n  web:\n    image: a\n    command:\n    - a\n", "#\n #\n", ""),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 64 * megabyte,
		},
		"and the same, refused for a key it does not take, read into nodes alone": {
			file: filled("x: 1\nservices:\n  web:\n    image: a\n    command:\n    - a\n", "#\n #\n", ""),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.x": stack.CodeUnsupportedKey}, invalid)
			},
			most: 32 * megabyte,
		},
		"a service whose command is a list in brackets of as many words as fit, each with a comment after it, and one more on a line of its own": {
			file: filled("services:\n  web:\n    image: a\n    command: [", "a #\n, #\n", "a]\n"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Empty(t, invalid)
			},
			most: 48 * megabyte,
		},
		"a service named as long as a name may be, which depends on one so named, waited on with as many keys of one letter in braces as fit, which it does not take": {
			file: filled("services:\n  ? "+longest+"\n  : image: a\n    depends_on: {"+longest+": {k", ",k", "}}\n"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + longest + ".depends_on." + longest + ".k": stack.CodeUnsupportedKey}, invalid)
			},
			most: 32 * megabyte,
		},
		"and one given as many of them itself": {
			file: filled("services:\n  ? "+longest+"\n  : {image: a, k", ",k", "}\n"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + longest + ".k": stack.CodeUnsupportedKey}, invalid)
			},
			most: 32 * megabyte,
		},
		"and one whose limits are given as many": {
			file: filled("services:\n  ? "+longest+"\n  : image: a\n    resources: {limits: {k", ",k", "}}\n"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + longest + ".resources.limits.k": stack.CodeUnsupportedKey}, invalid)
			},
			most: 32 * megabyte,
		},
		"and one that joins one network as often as fit": {
			file: filled("networks: {n: }\nservices:\n  ? "+longest+"\n  : image: a\n    networks: {n", ",n", "}\n"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + longest + ".networks.n": stack.CodeDuplicateKey}, invalid)
			},
			most: 32 * megabyte,
		},
		"and one given one variable, with nothing, as often as fit": {
			file: filled("services:\n  ? "+longest+"\n  : image: a\n    environment: {a", ",a", "}\n"),
			invalid: func(t *testing.T, invalid domain.ValidationErrors) {
				assert.Equal(t, domain.ValidationErrors{"compose.services." + longest + ".environment.a": stack.CodeDuplicateKey}, invalid)
			},
			most: 32 * megabyte,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.LessOrEqual(t, len(tt.file), stack.MaxFile)

			// the reading of it held to the strict decode, before what reading
			// it takes is counted.
			stack.Parsed(t, tt.file)

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

// filled is a file as large as a stack's may be: head, then item as often as
// fits with tail after it.
func filled(head string, item string, tail string) string {
	var b strings.Builder

	b.WriteString(head)
	for b.Len()+len(item)+len(tail) <= stack.MaxFile {
		b.WriteString(item)
	}

	b.WriteString(tail)

	return b.String()
}

// tagDirectives is a file as large as a stack's may be of as many %TAG
// directives as fit, each of a handle of three characters of its own, before
// a document of one service that uses none of them.
func tagDirectives() string {
	const word = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

	document := "---\nservices:\n  web: {image: a}\n"

	var b strings.Builder

	for i := 0; i < len(word)*len(word)*len(word); i++ {
		line := "%TAG !" + string([]byte{word[i/(len(word)*len(word))], word[i/len(word)%len(word)], word[i%len(word)]}) + "! a\n"
		if b.Len()+len(line)+len(document) > stack.MaxFile {
			break
		}

		b.WriteString(line)
	}

	b.WriteString(document)

	return b.String()
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

// longKeyed is a file as large as a stack's may be: what prefix says, a
// service, and a key of 32 KiB, an explicit key's, which YAML holds to no
// length, whose mapping says entry as many times as fit.
func longKeyed(prefix string, entry string) string {
	var b strings.Builder

	b.WriteString(prefix + "services:\n  web: {image: a}\n? " + strings.Repeat("a", 32<<10) + "\n: {" + entry)
	for b.Len()+len(", "+entry+"}\n") <= stack.MaxFile {
		b.WriteString(", " + entry)
	}

	b.WriteString("}\n")

	return b.String()
}

// keyed is the key of the level at i of deepKeyed: as many characters as a
// key may have, each the level's letter.
func keyed(i int) string {
	return strings.Repeat(string(rune('a'+i%26)), stack.MaxKeyLength)
}

// keyedPath is the path deepKeyed's levels of keys are under.
func keyedPath(levels int) string {
	keys := make([]string, levels)
	for i := range keys {
		keys[i] = keyed(i)
	}

	return "compose." + strings.Join(keys, ".")
}

// deepKeyed is a file as large as a stack's may be: x, anchored, a service,
// and as many levels of keys as long as a key may be as levels says, each the
// one key of the mapping of the one before, the innermost of which says entry
// as many times as fit.
func deepKeyed(levels int, entry string) string {
	var b strings.Builder

	b.WriteString("x: &x a\nservices:\n  web: {image: a}\n? " + keyed(0) + "\n: ")
	for i := 1; i < levels; i++ {
		b.WriteString("{? " + keyed(i) + " : ")
	}

	b.WriteString("{" + entry)
	for b.Len()+len(", "+entry)+levels+len("\n") <= stack.MaxFile {
		b.WriteString(", " + entry)
	}

	b.WriteString(strings.Repeat("}", levels) + "\n")

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

			_, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    ports:\n      - "+tt.port+"\n")

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

			_, invalid := stack.Parsed(t, limited(tt.key, tt.value))

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

			file, invalid := stack.Parsed(t, limited("memory", written))
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

			_, invalid := stack.Parsed(t, mounted(tt.volume))

			assert.Equal(t, domain.ValidationErrors{"compose.services.web.volumes[0]": tt.code}, invalid)
		})
	}

	t.Run("takes a bind of an absolute path and a declared volume, read only or not", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    volumes:\n      - /srv:/srv\n      - data:/data:ro\n      - {type: bind, source: /etc/site, target: /etc/site, read_only: false}\nvolumes:\n  data:\n")
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

			_, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, domain.ValidationErrors{tt.path: "invalid_name"}, invalid)
		})
	}

	t.Run("takes names of letters, digits, dots, dashes and underscores, from a letter or a digit", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  a:\n    image: nginx\n    networks: [front.end_1]\n  2-web_v1.0:\n    image: nginx\nnetworks:\n  front.end_1:\n")
		require.Empty(t, invalid)

		assert.Equal(t, []string{"2-web_v1.0", "a"}, file.ServiceNames())
	})

	longest := strings.Repeat("a", stack.MaxNameLength)
	long := longest + "a"

	t.Run("takes names as long as a name may be, and what names them", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, strings.NewReplacer("NAME", longest).Replace(
			"services:\n  NAME:\n    image: nginx\n    networks: [NAME]\n    volumes: [NAME:/data, {type: volume, source: NAME, target: /backup}]\n"+
				"  web:\n    image: nginx\n    depends_on: [NAME]\n    networks: {NAME: }\n  api:\n    image: api\n    depends_on: {NAME: {condition: service_started}}\n"+
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

			_, invalid := stack.Parsed(t, tt.file)

			assert.Equal(t, domain.ValidationErrors{tt.path: "too_large"}, invalid)
		})
	}
}

func TestFile_Levels(t *testing.T) {
	t.Parallel()

	t.Run("a chain is a level a service", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    depends_on: [api]\n  api:\n    image: api\n    depends_on: [db]\n  db:\n    image: postgres\n")
		require.Empty(t, invalid)

		assert.Equal(t, [][]string{{"db"}, {"api"}, {"web"}}, file.Levels())
	})

	t.Run("what depends on nothing shares the first level, in the order of its names", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    depends_on: [db, cache]\n  db:\n    image: postgres\n  cache:\n    image: redis\n  worker:\n    image: worker\n")
		require.Empty(t, invalid)

		assert.Equal(t, [][]string{{"cache", "db", "worker"}, {"web"}}, file.Levels())
	})

	t.Run("services in a ring are in none, which a stack's plan refuses", func(t *testing.T) {
		t.Parallel()

		file, invalid := stack.Parsed(t, "services:\n  web:\n    image: nginx\n    depends_on: [api]\n  api:\n    image: api\n    depends_on: [web]\n  db:\n    image: postgres\n")
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
