package sdk

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVCPUs(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		cores float64
		want  uint8
	}{
		"no limit boots one":              {cores: 0, want: 1},
		"a fraction boots a whole one":    {cores: 0.25, want: 1},
		"exactly one is one":              {cores: 1, want: 1},
		"a fraction over is rounded up":   {cores: 1.5, want: 2},
		"float noise is still rounded up": {cores: 0.1 + 0.2 + 2, want: 3},
		"whole cores stay themselves":     {cores: 8, want: 8},
		"the most a sandbox can have":     {cores: 255, want: 255},
		"a negative boots one":            {cores: -2, want: 1},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := vcpus(tt.cores)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	for name, cores := range map[string]float64{
		"more than 255":   255.5,
		"not a number":    math.NaN(),
		"infinitely many": math.Inf(1),
	} {
		t.Run(name+" is refused", func(t *testing.T) {
			t.Parallel()

			_, err := vcpus(cores)
			assert.Error(t, err)
		})
	}
}

func TestMemoryMiB(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		bytes uint64
		cpus  uint8
		want  uint32
	}{
		"whole MiB stay themselves":   {bytes: 64 << 20, cpus: 1, want: 64},
		"a byte over is a MiB more":   {bytes: 64<<20 + 1, cpus: 1, want: 65},
		"less than a MiB is one":      {bytes: 1, cpus: 1, want: 1},
		"compose's 256M is 256":       {bytes: 268435456, cpus: 1, want: 256},
		"nothing is nothing":          {bytes: 0, cpus: 1, want: 0},
		"the most a sandbox can have": {bytes: math.MaxUint32 << 20, cpus: 1, want: math.MaxUint32},

		// guests in between do not boot
		"91 MiB boots":                                 {bytes: 91 << 20, cpus: 1, want: 91},
		"92 MiB does not, and is raised":               {bytes: 92 << 20, cpus: 1, want: 114},
		"100 MiB on one vCPU is raised":                {bytes: 100 << 20, cpus: 1, want: 114},
		"100 MiB on four vCPUs is raised further":      {bytes: 100 << 20, cpus: 4, want: 120},
		"113 MiB on one vCPU is raised to 114":         {bytes: 113 << 20, cpus: 1, want: 114},
		"114 MiB on one vCPU boots":                    {bytes: 114 << 20, cpus: 1, want: 114},
		"120 MiB on eight vCPUs is raised to 128":      {bytes: 120 << 20, cpus: 8, want: 128},
		"128 MiB on eight vCPUs boots":                 {bytes: 128 << 20, cpus: 8, want: 128},
		"130 MiB on sixteen vCPUs is raised to 144":    {bytes: 130 << 20, cpus: 16, want: 144},
		"150 MiB on thirty-two vCPUs is raised to 176": {bytes: 150 << 20, cpus: 32, want: 176},
		"200 MiB on thirty-two vCPUs boots":            {bytes: 200 << 20, cpus: 32, want: 200},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := memoryMiB(tt.bytes, tt.cpus)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.GreaterOrEqual(t, uint64(got), ceilMiB(tt.bytes), "never less than asked")
		})
	}

	t.Run("more than a sandbox can have is refused", func(t *testing.T) {
		t.Parallel()

		_, err := memoryMiB(math.MaxUint64, 1)
		assert.Error(t, err)
	})
}

func TestRootDiskMiB(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		limit uint64
		want  uint32
	}{
		// usable is about 0.96·N − 65 MiB, so each of these holds its limit
		"64 MiB":             {limit: 64 << 20, want: 142},
		"a byte over 64 MiB": {limit: 64<<20 + 1, want: 143},
		"1 MiB":              {limit: 1 << 20, want: 77},
		"1 GiB":              {limit: 1 << 30, want: 1142},
		"10 GiB":             {limit: 10 << 30, want: 10742},
		"a single byte":      {limit: 1, want: 77},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := rootDiskMiB(tt.limit)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			usable := 0.96*float64(got) - 65
			assert.GreaterOrEqual(t, usable, float64(ceilMiB(tt.limit)), "the root holds the limit")
			assert.GreaterOrEqual(t, got, uint32(minRootDiskMiB), "microsandbox can make the root")
		})
	}

	t.Run("more than a sandbox can have is refused", func(t *testing.T) {
		t.Parallel()

		_, err := rootDiskMiB(math.MaxUint64)
		assert.Error(t, err)
	})
}

func TestEnvMap(t *testing.T) {
	t.Parallel()

	t.Run("entries become a map", func(t *testing.T) {
		t.Parallel()

		got, err := envMap([]string{"A=1", "B=x=y", "EMPTY=", "A=2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"A": "2", "B": "x=y", "EMPTY": ""}, got)
	})

	t.Run("no entries are no map", func(t *testing.T) {
		t.Parallel()

		got, err := envMap(nil)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	for _, entry := range []string{"NOEQUALS", "=value"} {
		t.Run(entry+" is refused", func(t *testing.T) {
			t.Parallel()

			_, err := envMap([]string{"A=1", entry})
			assert.ErrorContains(t, err, entry)
		})
	}
}

func TestErrnoName(t *testing.T) {
	t.Parallel()

	number := func(n int) *int { return &n }

	testcases := map[string]struct {
		name   string
		number *int
		kind   string
		want   string
	}{
		"the agent's name wins":               {name: "ENOENT", number: number(13), kind: "permission_denied", want: "ENOENT"},
		"a number is named as Linux names it": {number: number(13), want: "EACCES"},
		"ELOOP is Linux's 40, not darwin's":   {number: number(40), want: "ELOOP"},
		"an unknown number stays a number":    {number: number(99), want: "errno 99"},
		"a kind stands in for the errno":      {kind: "not_executable", want: "ENOEXEC"},
		"an unknown kind is itself":           {kind: "spawn_failed", want: "spawn_failed"},
		"nothing is nothing":                  {want: ""},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, errnoName(tt.name, tt.number, tt.kind))
		})
	}
}

func TestRunning(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"starting", "running", "draining", "paused"} {
		assert.True(t, running(status), status)
	}

	for _, status := range []string{"created", "stopped", "crashed", ""} {
		assert.False(t, running(status), status)
	}
}

func TestRuntimeVersion(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0.7.6", runtimeVersion("msb 0.7.6\n"))
	assert.Equal(t, "0.7.6", runtimeVersion("0.7.6"))
	assert.Equal(t, "msb version 0.7.6", runtimeVersion(" msb version 0.7.6 "))
}

func TestLastLine(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "error: registry error: not found", lastLine([]byte("   ✗ Pulling x\nerror: registry error: not found\n\n")))
	assert.Equal(t, "", lastLine(nil))
}
