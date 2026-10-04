package image

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestConfigOf(t *testing.T) {
	t.Parallel()

	t.Run("an image's own say in how it runs is kept, and its TCP ports in order", func(t *testing.T) {
		t.Parallel()

		config := configOf(v1.Config{
			Entrypoint:   []string{"/docker-entrypoint.sh"},
			Cmd:          []string{"nginx", "-g", "daemon off;"},
			Env:          []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			WorkingDir:   "/",
			User:         "101:101",
			ExposedPorts: map[string]struct{}{"80/tcp": {}, "443/tcp": {}, "53/udp": {}, "8080": {}, "0/tcp": {}, "http": {}},
		})

		assert.Equal(t, vm.ImageConfig{
			Entrypoint:   []string{"/docker-entrypoint.sh"},
			Cmd:          []string{"nginx", "-g", "daemon off;"},
			Env:          []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			WorkingDir:   "/",
			User:         "101:101",
			ExposedPorts: []uint16{80, 443, 8080},
		}, config)
	})
}

func TestDirName(t *testing.T) {
	t.Parallel()

	digest := "sha256:" + strings.Repeat("ab", 32)

	assert.Equal(t, "sha256-"+strings.Repeat("ab", 32), dirName(digest))

	back, ok := digestOf(dirName(digest))
	assert.True(t, ok)
	assert.Equal(t, digest, back, "a directory's name reads back as the digest it is called after")

	for _, other := range []string{"refs", ".build-sha256-abc", "sha256-abc", "sha256-" + strings.Repeat("AB", 32), "../sha256-" + strings.Repeat("ab", 32)} {
		_, ok := digestOf(other)
		assert.False(t, ok, other)
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	t.Run("entries are named relative to the root, and the root itself is left to sqfstar", func(t *testing.T) {
		t.Parallel()

		var layers bytes.Buffer
		writer := tar.NewWriter(&layers)

		entries := []tar.Header{
			{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "./bin/", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "./bin/busybox", Typeflag: tar.TypeReg, Mode: 0o755, Size: 4, Uid: 0},
			{Name: "./bin/sh", Typeflag: tar.TypeLink, Linkname: "./bin/busybox"},
			{Name: "bin/ls", Typeflag: tar.TypeSymlink, Linkname: "./busybox"},
			{Name: "/etc/passwd", Typeflag: tar.TypeReg, Mode: 0o644, Size: 0, Uid: 0},
			{Name: "../../escape", Typeflag: tar.TypeReg, Mode: 0o644, Size: 0},
		}

		for _, entry := range entries {
			require.NoError(t, writer.WriteHeader(&entry))

			if entry.Size > 0 {
				_, err := writer.Write([]byte("elf!"))
				require.NoError(t, err)
			}
		}
		require.NoError(t, writer.Close())

		var normalized bytes.Buffer
		require.NoError(t, normalize(&layers, &normalized))

		reader := tar.NewReader(&normalized)

		var names []string
		links := map[string]string{}
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)

			names = append(names, header.Name)
			if header.Linkname != "" {
				links[header.Name] = header.Linkname
			}

			if header.Name == "bin/busybox" {
				content, err := io.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, "elf!", string(content), "what an entry holds travels with it")
			}
		}

		assert.Equal(t, []string{"bin", "bin/busybox", "bin/sh", "bin/ls", "etc/passwd", "escape"}, names)
		assert.Equal(t, "bin/busybox", links["bin/sh"], "a hard link names its entry the same way")
		assert.Equal(t, "./busybox", links["bin/ls"], "a symlink says what it says")
	})
}

func TestRegistries(t *testing.T) {
	t.Parallel()

	t.Run("a registry is named as go-containerregistry names it, so docker.io is index.docker.io", func(t *testing.T) {
		t.Parallel()

		parsed, err := parseRegistries([]string{" GHCR.io ", "docker.io", "", "index.docker.io", "localhost:5000"})

		require.NoError(t, err)
		assert.Equal(t, []string{"ghcr.io", "index.docker.io", "localhost:5000"}, parsed)
	})

	t.Run("something that is not a registry is refused rather than allowing nothing", func(t *testing.T) {
		t.Parallel()

		_, err := parseRegistries([]string{"ghcr.io/tarhche"})

		assert.Error(t, err)
	})

	t.Run("an image is allowed from the registries named, and only from them", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{Registries: []string{"docker.io", "ghcr.io"}})

		for reference, allowed := range map[string]bool{
			"alpine":                         true,
			"docker.io/library/nginx:alpine": true,
			"ghcr.io/tarhche/code-runner:1":  true,
			"quay.io/prometheus/node":        false,
			"evil.example/ghcr.io/x":         false,
		} {
			ref, err := name.ParseReference(reference)
			require.NoError(t, err)

			err = store.allowed(ref)
			if allowed {
				assert.NoError(t, err, reference)
			} else {
				assert.ErrorIs(t, err, vm.ErrImage, reference)
			}
		}
	})

	t.Run("with no registries named, every one is allowed", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{})

		ref, err := name.ParseReference("quay.io/prometheus/node")
		require.NoError(t, err)

		assert.NoError(t, store.allowed(ref))
	})
}

func TestRefs(t *testing.T) {
	t.Parallel()

	store := newTestStore(t, Config{})
	digest := "sha256:" + strings.Repeat("cd", 32)

	written, err := name.ParseReference("alpine")
	require.NoError(t, err)
	require.NoError(t, store.remember(written, digest))

	t.Run("a reference is known under every spelling of it", func(t *testing.T) {
		for _, spelling := range []string{"alpine", "alpine:latest", "docker.io/library/alpine", "index.docker.io/library/alpine:latest"} {
			ref, err := name.ParseReference(spelling)
			require.NoError(t, err)

			known, found := store.known(ref)
			assert.True(t, found, spelling)
			assert.Equal(t, digest, known, spelling)
		}
	})

	t.Run("another reference is not", func(t *testing.T) {
		ref, err := name.ParseReference("alpine:3.20")
		require.NoError(t, err)

		_, found := store.known(ref)
		assert.False(t, found)
	})

	t.Run("a digest let go of takes its references with it", func(t *testing.T) {
		store.forget(digest)

		_, found := store.known(written)
		assert.False(t, found)
	})
}

func TestEnsure(t *testing.T) {
	t.Parallel()

	host, requests := serveRegistry(t)

	base := layer(t,
		entry{name: "etc/", typ: tar.TypeDir, mode: 0o755},
		entry{name: "etc/passwd", mode: 0o644, content: "root:x:0:0::/root:/bin/sh\n"},
		entry{name: "app/", typ: tar.TypeDir, mode: 0o755, uid: 1000, gid: 1000},
		entry{name: "app/old.txt", mode: 0o644, uid: 1000, gid: 1000, content: "old"},
		entry{name: "data/", typ: tar.TypeDir, mode: 0o755},
		entry{name: "data/a", mode: 0o644, content: "a"},
		entry{name: "data/b", mode: 0o644, content: "b"},
		entry{name: "tmp/", typ: tar.TypeDir, mode: 0o1777},
		entry{name: "tmp/gone", mode: 0o644, content: "gone"},
	)
	top := layer(t,
		entry{name: "app/.wh.old.txt", mode: 0o644},
		entry{name: "app/new.txt", mode: 0o640, uid: 1000, gid: 1000, content: "new"},
		entry{name: "data/.wh..wh..opq", mode: 0o644},
		entry{name: "data/c", mode: 0o644, content: "c"},
		entry{name: "tmp/.wh.gone", mode: 0o644},
	)

	config := v1.Config{
		Entrypoint:   []string{"/app/run"},
		Cmd:          []string{"--serve"},
		Env:          []string{"MODE=test"},
		WorkingDir:   "/app",
		User:         "1000",
		ExposedPorts: map[string]struct{}{"8080/tcp": {}},
	}

	image := makeImage(t, v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}, config, base, top)
	reference := host + "/tasks/app:1"
	push(t, reference, image)

	digest, err := image.Digest()
	require.NoError(t, err)

	store := newTestStore(t, Config{})
	squashes := countSquashes(store)

	clock := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }

	built, err := store.Ensure(context.Background(), reference)
	require.NoError(t, err)

	t.Run("an image is pulled for the host and made into a disk, with what it says about how it runs", func(t *testing.T) {
		assert.Equal(t, digest.String(), built.Digest)
		assert.Equal(t, reference, built.Reference)
		assert.Equal(t, filepath.Join(store.config.Dir, dirName(digest.String()), rootName), built.Root)
		assert.Equal(t, clock, built.LastUsedAt)
		assert.Equal(t, vm.ImageConfig{
			Entrypoint:   []string{"/app/run"},
			Cmd:          []string{"--serve"},
			Env:          []string{"MODE=test"},
			WorkingDir:   "/app",
			User:         "1000",
			ExposedPorts: []uint16{8080},
		}, built.Config)

		info, err := os.Stat(built.Root)
		require.NoError(t, err)
		assert.Equal(t, info.Size(), built.Size)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "every machine reads the image as a user of its own")
	})

	t.Run("its layers are laid over each other as a container runtime lays them", func(t *testing.T) {
		files := readTar(t, built.Root)

		assert.NotContains(t, files, "app/old.txt", "a file whited out above is gone")
		assert.NotContains(t, files, "tmp/gone", "a file whited out above is gone")
		assert.NotContains(t, files, "data/a", "an opaque directory hides what is below it")
		assert.NotContains(t, files, "data/b", "an opaque directory hides what is below it")
		assert.Contains(t, files, "data/c", "and keeps what is in it")
		assert.Contains(t, files, "etc/passwd")

		for path := range files {
			assert.NotContains(t, path, ".wh.", "no whiteout is carried into the disk")
		}

		assert.Equal(t, 1000, files["app/new.txt"].Uid, "whose a file is is kept")
		assert.Equal(t, 1000, files["app/new.txt"].Gid, "whose a file is is kept")
		assert.Equal(t, int64(0o640), files["app/new.txt"].Mode&0o7777, "and its mode")
		assert.Equal(t, int64(0o1777), files["tmp"].Mode&0o7777, "and a directory's")
	})

	t.Run("an image already here is not pulled again, under any spelling of its reference", func(t *testing.T) {
		before := requests.Load()
		clock = clock.Add(time.Hour)

		again, err := store.Ensure(context.Background(), reference)
		require.NoError(t, err)

		assert.Equal(t, built.Digest, again.Digest)
		assert.Equal(t, clock, again.LastUsedAt, "being asked for is being used")
		assert.Equal(t, before, requests.Load(), "the registry is not asked")

		respelled, err := store.Ensure(context.Background(), host+"/tasks/app:1@"+digest.String())
		require.NoError(t, err)
		assert.Equal(t, built.Digest, respelled.Digest)
		assert.Equal(t, int32(1), squashes.Load(), "one digest is made into a disk once")
	})

	t.Run("an image is asked for by its digest alone once it is here", func(t *testing.T) {
		before := requests.Load()

		byDigest, err := store.Ensure(context.Background(), digest.String())
		require.NoError(t, err)

		assert.Equal(t, built.Root, byDigest.Root)
		assert.Equal(t, host+"/tasks/app:1@"+digest.String(), byDigest.Reference, "the reference it was last asked for by is kept")
		assert.Equal(t, before, requests.Load())

		_, err = store.Ensure(context.Background(), "sha256:"+strings.Repeat("0", 64))
		assert.ErrorIs(t, err, vm.ErrImage, "a digest that is not here says nothing about where it could be pulled from")
	})

	t.Run("images here are listed, and one let go of is pulled again when asked for", func(t *testing.T) {
		listed, err := store.List(context.Background())
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, built.Digest, listed[0].Digest)

		require.NoError(t, store.Remove(context.Background(), built.Digest))

		_, err = os.Stat(filepath.Dir(built.Root))
		assert.ErrorIs(t, err, os.ErrNotExist)

		listed, err = store.List(context.Background())
		require.NoError(t, err)
		assert.Empty(t, listed)

		assert.ErrorIs(t, store.Remove(context.Background(), built.Digest), vm.ErrNotFound)
		assert.ErrorIs(t, store.Remove(context.Background(), "../../etc"), vm.ErrInvalid)

		before := requests.Load()

		pulled, err := store.Ensure(context.Background(), reference)
		require.NoError(t, err)
		assert.Equal(t, built.Digest, pulled.Digest)
		assert.Greater(t, requests.Load(), before, "its reference went with it, so the registry is asked again")
		assert.Equal(t, int32(2), squashes.Load())
	})
}

func TestEnsureRefusals(t *testing.T) {
	t.Parallel()

	host, requests := serveRegistry(t)

	t.Run("an image for another platform is refused, and nothing is made of it", func(t *testing.T) {
		t.Parallel()

		other := "s390x"
		if goruntime.GOARCH == other {
			other = "ppc64le"
		}

		reference := host + "/tasks/foreign:1"
		push(t, reference, makeImage(t, v1.Platform{OS: "linux", Architecture: other}, v1.Config{Cmd: []string{"true"}}, layer(t, entry{name: "x", mode: 0o644})))

		store := newTestStore(t, Config{})
		squashes := countSquashes(store)

		_, err := store.Ensure(context.Background(), reference)

		assert.ErrorIs(t, err, vm.ErrImage)
		assert.ErrorContains(t, err, "no variant for linux/"+goruntime.GOARCH)
		assert.Zero(t, squashes.Load())
	})

	t.Run("an index without the host's platform is refused", func(t *testing.T) {
		t.Parallel()

		other := makeImage(t, v1.Platform{OS: "linux", Architecture: "riscv64"}, v1.Config{}, layer(t, entry{name: "x", mode: 0o644}))
		index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
			Add:        other,
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "riscv64"}},
		})

		reference := host + "/tasks/multi:1"
		ref, err := name.ParseReference(reference)
		require.NoError(t, err)
		require.NoError(t, remote.WriteIndex(ref, index))

		store := newTestStore(t, Config{})

		_, err = store.Ensure(context.Background(), reference)

		assert.ErrorIs(t, err, vm.ErrImage)
	})

	t.Run("an index is asked for the host's platform", func(t *testing.T) {
		t.Parallel()

		mine := makeImage(t, v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}, v1.Config{Cmd: []string{"mine"}}, layer(t, entry{name: "mine", mode: 0o644}))
		theirs := makeImage(t, v1.Platform{OS: "linux", Architecture: "riscv64"}, v1.Config{Cmd: []string{"theirs"}}, layer(t, entry{name: "theirs", mode: 0o644}))
		index := mutate.AppendManifests(empty.Index,
			mutate.IndexAddendum{Add: theirs, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "riscv64"}}},
			mutate.IndexAddendum{Add: mine, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}}},
		)

		reference := host + "/tasks/both:1"
		ref, err := name.ParseReference(reference)
		require.NoError(t, err)
		require.NoError(t, remote.WriteIndex(ref, index))

		store := newTestStore(t, Config{})

		built, err := store.Ensure(context.Background(), reference)
		require.NoError(t, err)

		digest, err := mine.Digest()
		require.NoError(t, err)

		assert.Equal(t, digest.String(), built.Digest)
		assert.Equal(t, []string{"mine"}, built.Config.Cmd)
	})

	t.Run("an image from a registry images may not come from is refused before anything is asked of it", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{Registries: []string{"ghcr.io"}})

		before := requests.Load()

		_, err := store.Ensure(context.Background(), host+"/tasks/app:1")

		assert.ErrorIs(t, err, vm.ErrImage)
		assert.ErrorContains(t, err, "images may only come from ghcr.io")
		assert.Equal(t, before, requests.Load())
	})

	t.Run("an image that cannot be pulled says so", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{})

		_, err := store.Ensure(context.Background(), host+"/tasks/nothing:1")

		assert.ErrorIs(t, err, vm.ErrImage)
	})

	t.Run("something that is not a reference is refused", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{})

		_, err := store.Ensure(context.Background(), "Not A Reference!")

		assert.ErrorIs(t, err, vm.ErrInvalid)
	})

	t.Run("an image that cannot be made into a disk leaves nothing behind", func(t *testing.T) {
		t.Parallel()

		reference := host + "/tasks/broken:1"
		push(t, reference, makeImage(t, v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}, v1.Config{}, layer(t, entry{name: "x", mode: 0o644, content: "x"})))

		store := newTestStore(t, Config{})
		store.squash = func(ctx context.Context, path string, write func(io.Writer) error) error {
			require.NoError(t, os.WriteFile(path, []byte("half"), 0o600))

			return errors.New("sqfstar failed: no space left on device")
		}

		_, err := store.Ensure(context.Background(), reference)

		assert.ErrorIs(t, err, vm.ErrImage)
		assert.ErrorContains(t, err, "no space left on device")

		entries, err := os.ReadDir(store.config.Dir)
		require.NoError(t, err)

		for _, entry := range entries {
			assert.False(t, strings.HasPrefix(entry.Name(), buildPrefix), "no build is left behind: %s", entry.Name())
			_, isImage := digestOf(entry.Name())
			assert.False(t, isImage, "no image is left behind: %s", entry.Name())
		}
	})
}

func TestEnsureConcurrently(t *testing.T) {
	t.Parallel()

	host, _ := serveRegistry(t)

	reference := host + "/tasks/popular:1"
	push(t, reference, makeImage(t, v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}, v1.Config{}, layer(t, entry{name: "x", mode: 0o644, content: "x"})))

	store := newTestStore(t, Config{})
	squashes := countSquashes(store)

	var (
		wait    sync.WaitGroup
		digests sync.Map
	)

	for i := range 8 {
		wait.Go(func() {
			built, err := store.Ensure(context.Background(), reference)
			if assert.NoError(t, err) {
				digests.Store(i, built.Digest)
			}
		})
	}

	wait.Wait()

	seen := map[any]bool{}
	digests.Range(func(_ any, digest any) bool {
		seen[digest] = true

		return true
	})

	assert.Len(t, seen, 1)
	assert.Equal(t, int32(1), squashes.Load(), "one digest asked for at once by many is made into a disk once")
}

func TestPrune(t *testing.T) {
	t.Parallel()

	host, _ := serveRegistry(t)

	store := newTestStore(t, Config{})
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	clock := now
	store.now = func() time.Time { return clock }

	ensure := func(tag string, at time.Time) vm.Image {
		t.Helper()

		reference := host + "/tasks/prune:" + tag
		push(t, reference, makeImage(t, v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}, v1.Config{Cmd: []string{tag}}, layer(t, entry{name: tag, mode: 0o644, content: strings.Repeat(tag, 4096)})))

		clock = at
		built, err := store.Ensure(context.Background(), reference)
		require.NoError(t, err)

		return built
	}

	oldest := ensure("a", now.Add(-3*time.Hour))
	kept := ensure("b", now.Add(-2*time.Hour))
	older := ensure("c", now.Add(-time.Hour))
	recent := ensure("d", now.Add(-time.Minute))

	clock = now

	t.Run("without a limit, nothing is let go", func(t *testing.T) {
		removed, err := store.Prune(context.Background(), nil)

		require.NoError(t, err)
		assert.Empty(t, removed)
	})

	t.Run("the least recently used that nothing boots are let go until the rest fit", func(t *testing.T) {
		store.config.CacheMax = uint64(kept.Size + recent.Size + older.Size/2)

		removed, err := store.Prune(context.Background(), []string{kept.Digest})
		require.NoError(t, err)

		var digests []string
		for _, image := range removed {
			digests = append(digests, image.Digest)
		}

		assert.Equal(t, []string{oldest.Digest, older.Digest}, digests)

		listed, err := store.List(context.Background())
		require.NoError(t, err)

		var left []string
		for _, image := range listed {
			left = append(left, image.Digest)
		}

		assert.Equal(t, []string{recent.Digest, kept.Digest}, left, "the most recently used first")
	})
}

func TestEvictable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	before := now.Add(-pruneGrace)

	image := func(digest string, size int64, used time.Duration) vm.Image {
		return vm.Image{Digest: digest, Size: size, LastUsedAt: now.Add(-used)}
	}

	images := []vm.Image{
		image("fresh", 100, time.Minute),
		image("old", 100, 3*time.Hour),
		image("kept", 100, 4*time.Hour),
		image("older", 100, 5*time.Hour),
		{Digest: "never", Size: 100},
	}

	digests := func(images []vm.Image) []string {
		var result []string
		for _, image := range images {
			result = append(result, image.Digest)
		}

		return result
	}

	t.Run("nothing is let go while everything fits", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, evictable(images, nil, 500, before))
	})

	t.Run("the least recently used go first, one never used before any", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{"never", "older"}, digests(evictable(images, []string{"kept"}, 300, before)))
	})

	t.Run("what is kept and what was just used stay, even if the rest cannot fit", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{"never", "older", "old"}, digests(evictable(images, []string{"kept"}, 0, before)))
	})
}

func TestMakeScratch(t *testing.T) {
	t.Parallel()

	t.Run("a scratch disk is made beside where it goes, formatted, and moved there whole", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{})

		var formatted string
		store.format = func(ctx context.Context, path string) error {
			formatted = path

			return nil
		}

		path := filepath.Join(t.TempDir(), "vms", "0123456789abcdef", "scratch.ext4")

		require.NoError(t, store.MakeScratch(context.Background(), path, 64<<20))

		info, err := os.Stat(path)
		require.NoError(t, err)

		assert.Equal(t, int64(64<<20), info.Size())
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the disk is vmhost's until a machine is given it")
		assert.Equal(t, filepath.Dir(path), filepath.Dir(formatted))
		assert.NotEqual(t, path, formatted)

		assertOnly(t, filepath.Dir(path), "scratch.ext4")
	})

	t.Run("a disk that cannot be formatted is not left behind", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{})
		store.format = func(ctx context.Context, path string) error {
			return errors.New("mke2fs failed")
		}

		path := filepath.Join(t.TempDir(), "scratch.ext4")

		assert.Error(t, store.MakeScratch(context.Background(), path, 64<<20))
		assertOnly(t, filepath.Dir(path))
	})

	t.Run("a disk of no size is refused", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t, Config{})

		assert.ErrorIs(t, store.MakeScratch(context.Background(), filepath.Join(t.TempDir(), "scratch.ext4"), 0), vm.ErrInvalid)
		assert.ErrorIs(t, store.MakeScratch(context.Background(), "scratch.ext4", 64<<20), vm.ErrInvalid)
	})

	t.Run("mke2fs makes an ext4 disk that takes next to nothing until it is written", func(t *testing.T) {
		t.Parallel()

		mke2fs, err := exec.LookPath("mke2fs")
		if err != nil {
			t.Skip("mke2fs is not here")
		}

		store := newTestStore(t, Config{})
		store.format = formatWith(mke2fs)

		path := filepath.Join(t.TempDir(), "scratch.ext4")
		require.NoError(t, store.MakeScratch(context.Background(), path, 1<<30))

		disk, err := os.Open(path)
		require.NoError(t, err)
		defer disk.Close()

		// an ext4 superblock starts 1024 bytes in, and says what it is 56
		// bytes into itself.
		magic := make([]byte, 2)
		_, err = disk.ReadAt(magic, 1024+56)
		require.NoError(t, err)
		assert.Equal(t, []byte{0x53, 0xef}, magic)

		label := make([]byte, 16)
		_, err = disk.ReadAt(label, 1024+120)
		require.NoError(t, err)
		assert.Equal(t, "scratch", strings.TrimRight(string(label), "\x00"))

		info, err := disk.Stat()
		require.NoError(t, err)
		assert.Equal(t, int64(1<<30), info.Size())
		assert.Less(t, allocated(info), int64(4<<20), "a 1 GiB disk nobody wrote to takes next to nothing on the host")
	})
}

func TestSweep(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	for _, leftover := range []string{trashPrefix + "1", buildPrefix + "fresh", buildPrefix + "stale"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, leftover, "inside"), 0o700))
	}

	past := time.Now().Add(-2 * staleBuild)
	require.NoError(t, os.Chtimes(filepath.Join(dir, buildPrefix+"stale"), past, past))

	_, err := newStore(Config{Dir: dir}, slog.New(slog.DiscardHandler), nil, nil)
	require.NoError(t, err)

	assertOnly(t, dir, buildPrefix+"fresh", refsDir)
}

func TestSquashfs(t *testing.T) {
	t.Parallel()

	sqfstar, err := exec.LookPath("sqfstar")
	if err != nil {
		t.Skip("sqfstar is not here")
	}

	unsquashfs, err := exec.LookPath("unsquashfs")
	if err != nil {
		t.Skip("unsquashfs is not here")
	}

	host, _ := serveRegistry(t)

	reference := host + "/tasks/squashed:1"
	push(t, reference, makeImage(t, v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}, v1.Config{},
		layer(t,
			entry{name: "./", typ: tar.TypeDir, mode: 0o755},
			entry{name: "./app/", typ: tar.TypeDir, mode: 0o750, uid: 1000, gid: 1000},
			entry{name: "./app/old.txt", mode: 0o644, uid: 1000, gid: 1000, content: "old"},
			entry{name: "./bin/", typ: tar.TypeDir, mode: 0o755},
			entry{name: "./bin/busybox", mode: 0o755, content: "elf!"},
			entry{name: "./bin/sh", typ: tar.TypeLink, link: "./bin/busybox"},
		),
		layer(t,
			entry{name: "app/.wh.old.txt", mode: 0o644},
			entry{name: "app/new.txt", mode: 0o600, uid: 1000, gid: 1000, content: "new"},
		),
	))

	store := newTestStore(t, Config{})
	store.squash = squashWith(sqfstar)

	built, err := store.Ensure(context.Background(), reference)
	require.NoError(t, err)

	listing, err := exec.Command(unsquashfs, "-lln", built.Root).CombinedOutput()
	require.NoError(t, err, string(listing))

	lines := map[string]string{}
	for _, line := range strings.Split(string(listing), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		lines[strings.TrimPrefix(fields[len(fields)-1], "squashfs-root")] = fields[0] + " " + fields[1]
	}

	assert.Equal(t, "drwxr-xr-x 0/0", lines[""], "the root is root's")
	assert.Equal(t, "drwxr-x--- 1000/1000", lines["/app"], "whose a directory is is kept, and its mode")
	assert.Equal(t, "-rw------- 1000/1000", lines["/app/new.txt"], "whose a file is is kept, and its mode")
	assert.NotContains(t, lines, "/app/old.txt", "a file whited out above is gone")
	assert.NotContains(t, lines, "/app/.wh.old.txt", "and so is its whiteout")
	assert.Equal(t, "-rwxr-xr-x 0/0", lines["/bin/sh"], "a hard link is the file it links to")
}

// entry is one entry of a layer.
type entry struct {
	name     string
	typ      byte
	mode     int64
	uid, gid int
	content  string
	link     string
}

// layer is a layer holding entries, as a tarball.
func layer(t *testing.T, entries ...entry) v1.Layer {
	t.Helper()

	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)

	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}

		require.NoError(t, writer.WriteHeader(&tar.Header{
			Name:     e.name,
			Typeflag: typ,
			Mode:     e.mode,
			Uid:      e.uid,
			Gid:      e.gid,
			Size:     int64(len(e.content)),
			Linkname: e.link,
			ModTime:  time.Unix(1700000000, 0),
		}))

		_, err := writer.Write([]byte(e.content))
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())

	content := buffer.Bytes()

	built, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content)), nil
	})
	require.NoError(t, err)

	return built
}

// makeImage is an image for platform, made of layers.
func makeImage(t *testing.T, platform v1.Platform, config v1.Config, layers ...v1.Layer) v1.Image {
	t.Helper()

	made, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: platform.OS, Architecture: platform.Architecture, Config: config})
	require.NoError(t, err)

	made, err = mutate.AppendLayers(made, layers...)
	require.NoError(t, err)

	return made
}

// serveRegistry serves a registry of its own for a test, and counts what is
// asked of it.
func serveRegistry(t *testing.T) (string, *atomic.Int64) {
	t.Helper()

	var requests atomic.Int64

	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://"), &requests
}

func push(t *testing.T, reference string, image v1.Image) {
	t.Helper()

	ref, err := name.ParseReference(reference)
	require.NoError(t, err)

	require.NoError(t, remote.Write(ref, image))
}

// newTestStore is a store in a directory of its own, reaching registries
// without credentials, which makes filesystems by writing the tarball it is
// given as it is.
func newTestStore(t *testing.T, config Config) *Store {
	t.Helper()

	config.Dir = filepath.Join(t.TempDir(), "images")
	require.NoError(t, os.MkdirAll(config.Dir, 0o700))

	store, err := newStore(config, slog.New(slog.DiscardHandler), copyTarball, nil)
	require.NoError(t, err)

	store.keychain = authn.NewMultiKeychain()

	return store
}

// copyTarball stands in for sqfstar: the filesystem it makes is the tarball.
func copyTarball(ctx context.Context, path string, write func(io.Writer) error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	return errors.Join(write(file), file.Close())
}

// countSquashes counts the filesystems a store makes.
func countSquashes(store *Store) *atomic.Int32 {
	var count atomic.Int32

	squash := store.squash
	store.squash = func(ctx context.Context, path string, write func(io.Writer) error) error {
		count.Add(1)

		return squash(ctx, path, write)
	}

	return &count
}

// readTar reads back the tarball copyTarball made, by name.
func readTar(t *testing.T, path string) map[string]*tar.Header {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	headers := map[string]*tar.Header{}
	reader := tar.NewReader(file)

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return headers
		}
		require.NoError(t, err)

		headers[header.Name] = header
	}
}

// assertOnly asserts that a directory holds the entries named and nothing else.
func assertOnly(t *testing.T, dir string, names ...string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var found []string
	for _, entry := range entries {
		found = append(found, entry.Name())
	}

	assert.ElementsMatch(t, names, found)
}
