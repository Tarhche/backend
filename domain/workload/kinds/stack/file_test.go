package stack_test

import (
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
		"a merge of another's":          {file: "x-base: &base\n  image: nginx\nservices:\n  web:\n    <<: *base\n", path: "compose.x-base"},
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
