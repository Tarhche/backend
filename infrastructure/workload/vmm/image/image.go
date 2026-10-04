// Package image makes the OCI images tasks name into disks microVMs boot, and
// makes the scratch disks writable tasks keep their changes on.
//
// A task names an image the way it would for a container: nginx:alpine. The
// image is pulled for the host's platform with go-containerregistry, its layers
// are laid over each other the way a container runtime lays them (whiteouts
// and opaque directories applied), and the result is streamed as a tarball
// straight into sqfstar, which writes a zstd-compressed squashfs of it, keeping
// whose each file is without extracting anything. Squashfs is read only by
// design, which is all an image is ever asked to be: every VM running the
// image shares it, and what a VM changes goes to its scratch disk, which the
// guest lays over the image with overlayfs.
//
// An image is kept by its digest, so a tag that moves is a new image rather
// than a changed one, and one digest is built once however many ask for it at
// the same time. Under the store's directory (layout.Images):
//
//	sha256-<hex>/rootfs.squashfs   the image's root: read only, everybody's to read
//	sha256-<hex>/config.json       what the image says about how it is run
//	sha256-<hex>/usage.json        what it was last asked for by, and when
//	refs/<sha256 of a reference>   the digest a reference was last made under
//	.sha256-<hex>.lock             held while the digest is built or removed
//
// Which images VMs still boot is vmhost's to know, not the store's: it lets go
// of an image when asked (Remove), and of the least recently used ones that
// nothing boots once they take more disk than they may (Prune).
//
// Ported from PR #101's image store, which was tested end to end; what is new
// is the registry allowlist, the check that an image is for the host's
// platform, the cache's bookkeeping and the bare-digest lookup.
package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

const (
	rootName   = "rootfs.squashfs"
	configName = "config.json"
	usageName  = "usage.json"
	refsDir    = "refs"

	// buildPrefix and trashPrefix start what is on its way in and on its way
	// out: an image is built in a directory of its own and moved into place
	// whole, and moved out of place whole before it is deleted, so a reader
	// never finds half of one.
	buildPrefix = ".build-"
	trashPrefix = ".trash-"

	// staleBuild is how old a build left behind has to be to be taken for
	// one whose maker went away, rather than one still being made.
	staleBuild = time.Hour
)

// Config is where images are kept, and which ones may be.
type Config struct {
	// Dir is where images' disks are kept (layout.Images).
	Dir string

	// Registries are the registries images may come from, as their hosts
	// name them: ghcr.io, docker.io. None is any.
	Registries []string

	// CacheMax is how much disk, in bytes, images may take before the least
	// recently used that nothing boots are let go (Prune). Zero is no limit.
	CacheMax uint64
}

// Store keeps images' disks.
type Store struct {
	config Config
	logger *slog.Logger
	tracer oteltrace.Tracer

	// registries are the registries images may come from, as
	// go-containerregistry names them, so that docker.io and index.docker.io
	// are one.
	registries []string

	// keychain is where credentials for a registry come from: the docker
	// config file mounted into vmhost.
	keychain authn.Keychain

	// squash makes a filesystem out of a tarball, and format makes an empty
	// filesystem on a disk: sqfstar and mke2fs, which tests stand in for.
	squash squasher
	format formatter

	now func() time.Time

	// builds makes one build of a digest serve everybody asking for it at
	// once in this process; a lock file does the same across processes.
	builds singleflight.Group
}

var _ vm.ImageStore = (*Store)(nil)

// NewStore keeps images' disks as config says.
//
// It refuses to start without sqfstar (squashfs-tools 4.6 or later) or mke2fs
// (e2fsprogs): vmhost cannot make a VM's disks without them, and is better
// told so when it starts than when the first task does.
func NewStore(config Config, logger *slog.Logger) (*Store, error) {
	sqfstar, err := exec.LookPath("sqfstar")
	if err != nil {
		return nil, fmt.Errorf("images cannot be made into disks without sqfstar (squashfs-tools 4.6 or later): %w", err)
	}

	mke2fs, err := exec.LookPath("mke2fs")
	if err != nil {
		return nil, fmt.Errorf("scratch disks cannot be made without mke2fs (e2fsprogs): %w", err)
	}

	return newStore(config, logger, squashWith(sqfstar), formatWith(mke2fs))
}

// newStore is NewStore with what makes filesystems given.
func newStore(config Config, logger *slog.Logger, squash squasher, format formatter) (*Store, error) {
	if !filepath.IsAbs(config.Dir) {
		return nil, fmt.Errorf("%q is not a directory images can be kept in: it has to be an absolute path", config.Dir)
	}

	registries, err := parseRegistries(config.Registries)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Join(config.Dir, refsDir), 0o700); err != nil {
		return nil, err
	}

	s := &Store{
		config:     config,
		logger:     logger,
		tracer:     otel.Tracer("workload.vmhost.image"),
		registries: registries,
		keychain:   authn.DefaultKeychain,
		squash:     squash,
		format:     format,
		now:        time.Now,
	}

	s.sweep()

	return s, nil
}

// Ensure makes sure an image is here, pulling and building it if it is not.
//
// An image already built under the reference it was asked by is taken as it
// is, without asking its registry again, the same as a container runtime,
// which does not pull an image it already holds. A reference is held the way
// go-containerregistry names it, so alpine and docker.io/library/alpine:latest
// are one reference.
//
// A bare digest (sha256:…) is an image built here already, which is how a VM
// made from an image boots it again after its tag has moved: a digest alone
// says nothing about where it could be pulled from.
//
// Every image is asked for by a VM about to be made from it, so being asked
// for is what keeps it from being let go (LastUsedAt).
func (s *Store) Ensure(ctx context.Context, reference string) (vm.Image, error) {
	ctx, span := s.tracer.Start(ctx, "vmhost.image.ensure",
		oteltrace.WithAttributes(attribute.String("image", reference)),
	)
	defer span.End()

	image, err := s.ensure(ctx, reference)

	return image, trace.RecordError(span, err)
}

func (s *Store) ensure(ctx context.Context, reference string) (vm.Image, error) {
	if digest, err := v1.NewHash(reference); err == nil {
		return s.ensureBuilt(digest.String())
	}

	ref, err := name.ParseReference(reference)
	if err != nil {
		return vm.Image{}, fmt.Errorf("%w: %q is not an image: %w", vm.ErrInvalid, reference, err)
	}

	if err := s.allowed(ref); err != nil {
		return vm.Image{}, err
	}

	if digest, found := s.known(ref); found {
		if built, err := s.load(digest); err == nil {
			return s.use(built, reference), nil
		}
	}

	pulled, err := remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(s.keychain),
		remote.WithPlatform(v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}),
	)
	if err != nil {
		return vm.Image{}, fmt.Errorf("%w: %s cannot be pulled: %w", vm.ErrImage, reference, err)
	}

	file, err := pulled.ConfigFile()
	if err != nil {
		return vm.Image{}, fmt.Errorf("%w: what %s says about itself cannot be read: %w", vm.ErrImage, reference, err)
	}

	// an index is asked for the host's platform, but an image that is not an
	// index is handed over whatever it is for, and would only fail once a VM
	// runs it: told now, it is told why.
	if !forHost(file) {
		return vm.Image{}, fmt.Errorf("%w: %s has no variant for linux/%s: it is for %s/%s", vm.ErrImage, reference, goruntime.GOARCH, file.OS, file.Architecture)
	}

	digest, err := pulled.Digest()
	if err != nil {
		return vm.Image{}, fmt.Errorf("%w: %s: %w", vm.ErrImage, reference, err)
	}

	built, err, _ := s.builds.Do(digest.String(), func() (any, error) {
		return s.build(ctx, reference, digest, pulled, file.Config)
	})
	if err != nil {
		return vm.Image{}, err
	}

	// a reference not written down only sends the next ask for it to its
	// registry again; the image is here either way.
	if err := s.remember(ref, digest.String()); err != nil {
		s.logger.Warn("failed to write down the digest a reference was made under", "image", reference, "digest", digest.String(), "error", err)
	}

	return s.use(built.(vm.Image), reference), nil
}

// ensureBuilt is an image built here already, asked for by its digest. The
// registry it came from has to be one images may still come from.
func (s *Store) ensureBuilt(digest string) (vm.Image, error) {
	built, err := s.load(digest)
	if errors.Is(err, os.ErrNotExist) {
		return vm.Image{}, fmt.Errorf("%w: %s is not here, and a digest alone does not say where it could be pulled from", vm.ErrImage, digest)
	}

	if err != nil {
		return vm.Image{}, err
	}

	if ref, err := name.ParseReference(built.Reference); err == nil {
		if err := s.allowed(ref); err != nil {
			return vm.Image{}, err
		}
	}

	return s.use(built, ""), nil
}

// forHost reports whether an image runs on the host: built for Linux and for
// the host's architecture. An image that does not say is taken at its word.
func forHost(file *v1.ConfigFile) bool {
	if file.OS != "" && file.OS != "linux" {
		return false
	}

	return file.Architecture == "" || file.Architecture == goruntime.GOARCH
}

// build makes a pulled image into a disk, unless another process already has.
func (s *Store) build(ctx context.Context, reference string, digest v1.Hash, pulled v1.Image, config v1.Config) (vm.Image, error) {
	ctx, span := s.tracer.Start(ctx, "vmhost.image.build",
		oteltrace.WithAttributes(attribute.String("image", reference), attribute.String("digest", digest.String())),
	)
	defer span.End()

	unlock, err := lock(s.lockPath(digest.String()))
	if err != nil {
		return vm.Image{}, trace.RecordError(span, err)
	}
	defer unlock()

	if built, err := s.load(digest.String()); err == nil {
		return built, nil
	}

	s.logger.Info("image is not here, pulling it", "image", reference, "digest", digest.String())

	started := s.now()

	work, err := os.MkdirTemp(s.config.Dir, buildPrefix+dirName(digest.String())+"-")
	if err != nil {
		return vm.Image{}, trace.RecordError(span, err)
	}
	defer os.RemoveAll(work)

	// a temporary directory is made for its maker alone; an image's is gone
	// through by whatever reads it.
	if err := os.Chmod(work, 0o755); err != nil {
		return vm.Image{}, trace.RecordError(span, err)
	}

	root := filepath.Join(work, rootName)
	if err := s.makeFilesystem(ctx, pulled, root); err != nil {
		return vm.Image{}, trace.RecordError(span, fmt.Errorf("%w: %s cannot be made into a disk: %w", vm.ErrImage, reference, err))
	}

	// every machine that boots the image reads it as a user of its own, so it
	// is everybody's to read, whatever sqfstar made it; the hypervisor
	// refuses to share a disk that is not.
	if err := os.Chmod(root, 0o644); err != nil {
		return vm.Image{}, trace.RecordError(span, err)
	}

	if err := writeJSON(filepath.Join(work, configName), configOf(config)); err != nil {
		return vm.Image{}, trace.RecordError(span, err)
	}

	// moved into place whole, so an image is either there or it is not.
	if err := os.Rename(work, s.imageDir(digest.String())); err != nil {
		// a store holding no lock — one from before a restart, say — may
		// have got there first, which is as good.
		if built, loaded := s.load(digest.String()); loaded == nil {
			return built, nil
		}

		return vm.Image{}, trace.RecordError(span, err)
	}

	built, err := s.load(digest.String())
	if err != nil {
		return vm.Image{}, trace.RecordError(span, err)
	}

	s.logger.Info("image pulled", "image", reference, "digest", built.Digest, "size", built.Size, "took", s.now().Sub(started).String())

	return built, nil
}

// makeFilesystem writes an image's layers, laid over each other, into a
// squashfs filesystem at path. The layers travel as a tarball straight into
// sqfstar, which keeps whose each file is as the tarball says it: extracting
// them first would lose that, unless whoever did it were root.
func (s *Store) makeFilesystem(ctx context.Context, img v1.Image, path string) error {
	layers := mutate.Extract(img)
	defer layers.Close()

	return s.squash(ctx, path, func(w io.Writer) error {
		return normalize(layers, w)
	})
}

// List is every image built here, the most recently used first.
func (s *Store) List(ctx context.Context) ([]vm.Image, error) {
	entries, err := os.ReadDir(s.config.Dir)
	if err != nil {
		return nil, err
	}

	images := make([]vm.Image, 0, len(entries))

	for _, entry := range entries {
		digest, ok := digestOf(entry.Name())
		if !ok || !entry.IsDir() {
			continue
		}

		built, err := s.load(digest)
		if err != nil {
			// a directory that is not a whole image is one on its way out.
			continue
		}

		images = append(images, built)
	}

	slices.SortFunc(images, func(a vm.Image, b vm.Image) int {
		if order := b.LastUsedAt.Compare(a.LastUsedAt); order != 0 {
			return order
		}

		return strings.Compare(a.Digest, b.Digest)
	})

	return images, nil
}

// Remove lets go of an image's disk, and of every reference made under it.
// A VM booted from it before keeps reading it until it ends: its disk is
// linked into the machine's own directory, which holds on to it. Whether a VM
// will boot it again is vmhost's to know, and to check first.
func (s *Store) Remove(ctx context.Context, digest string) error {
	hash, err := v1.NewHash(digest)
	if err != nil {
		return fmt.Errorf("%w: %q is not a digest: %w", vm.ErrInvalid, digest, err)
	}

	digest = hash.String()

	unlock, err := lock(s.lockPath(digest))
	if err != nil {
		return err
	}
	defer unlock()

	dir := s.imageDir(digest)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: no image %s", vm.ErrNotFound, digest)
	} else if err != nil {
		return err
	}

	// moved out of place whole first, into a directory of its own that is
	// on its way out, so nothing finds half of it while it is deleted.
	trash, err := os.MkdirTemp(s.config.Dir, trashPrefix)
	if err != nil {
		return err
	}

	if err := os.Rename(dir, filepath.Join(trash, filepath.Base(dir))); err != nil {
		return errors.Join(err, os.Remove(trash))
	}

	if err := os.RemoveAll(trash); err != nil {
		return err
	}

	s.forget(digest)

	s.logger.Info("image removed", "digest", digest)

	return nil
}

// load reads back an image that has been built.
func (s *Store) load(digest string) (vm.Image, error) {
	dir := s.imageDir(digest)

	var config vm.ImageConfig
	if err := readJSON(filepath.Join(dir, configName), &config); err != nil {
		return vm.Image{}, err
	}

	root := filepath.Join(dir, rootName)

	info, err := os.Stat(root)
	if err != nil {
		return vm.Image{}, err
	}

	built := vm.Image{Digest: digest, Root: root, Size: info.Size(), Config: config}

	// what it was asked for by, and when, is bookkeeping: an image without
	// it is still whole, and is simply the first to be let go.
	var use usage
	if err := readJSON(filepath.Join(dir, usageName), &use); err == nil {
		built.Reference = use.Reference
		built.LastUsedAt = use.LastUsedAt
	}

	return built, nil
}

// usage is what an image was last asked for by, and when.
type usage struct {
	Reference  string    `json:"reference,omitempty"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// use writes down that a VM is being made from an image, asked for by
// reference (empty keeps the one it had). Failing to is not failing to make
// the VM: it only leaves the image looking less used than it is.
func (s *Store) use(built vm.Image, reference string) vm.Image {
	if reference != "" {
		built.Reference = reference
	}

	built.LastUsedAt = s.now().UTC()

	if err := writeJSON(filepath.Join(s.imageDir(built.Digest), usageName), usage{Reference: built.Reference, LastUsedAt: built.LastUsedAt}); err != nil {
		s.logger.Warn("failed to write down that an image was used", "digest", built.Digest, "error", err)
	}

	return built
}

// known is the digest an image was last built under by reference.
func (s *Store) known(ref name.Reference) (string, bool) {
	content, err := os.ReadFile(s.refPath(ref))
	if err != nil {
		return "", false
	}

	digest, err := v1.NewHash(strings.TrimSpace(string(content)))
	if err != nil {
		return "", false
	}

	return digest.String(), true
}

// remember writes down the digest a reference was built under.
func (s *Store) remember(ref name.Reference, digest string) error {
	return writeFile(s.refPath(ref), []byte(digest))
}

// forget lets go of every reference made under a digest. A reference left
// behind would only send the next ask for it to pull again, so failing to is
// logged and nothing more.
func (s *Store) forget(digest string) {
	dir := filepath.Join(s.config.Dir, refsDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		s.logger.Warn("failed to read the references images were made under", "error", err)

		return
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		content, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(content)) != digest {
			continue
		}

		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("failed to let go of a reference", "digest", digest, "error", err)
		}
	}
}

// sweep takes away what a store that went away in the middle of something
// left behind: images half removed, and builds old enough to have been
// abandoned rather than still going.
func (s *Store) sweep() {
	entries, err := os.ReadDir(s.config.Dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		path := filepath.Join(s.config.Dir, entry.Name())

		switch {
		case strings.HasPrefix(entry.Name(), trashPrefix):
		case strings.HasPrefix(entry.Name(), buildPrefix):
			info, err := entry.Info()
			if err != nil || s.now().Sub(info.ModTime()) < staleBuild {
				continue
			}
		default:
			continue
		}

		if err := os.RemoveAll(path); err != nil {
			s.logger.Warn("failed to take away what was left of an image", "path", path, "error", err)
		}
	}
}

func (s *Store) refPath(ref name.Reference) string {
	sum := sha256.Sum256([]byte(ref.Name()))

	return filepath.Join(s.config.Dir, refsDir, hex.EncodeToString(sum[:]))
}

func (s *Store) imageDir(digest string) string {
	return filepath.Join(s.config.Dir, dirName(digest))
}

func (s *Store) lockPath(digest string) string {
	return filepath.Join(s.config.Dir, "."+dirName(digest)+".lock")
}

// configOf reads what an image says about how it is run.
func configOf(config v1.Config) vm.ImageConfig {
	ports := make([]uint16, 0, len(config.ExposedPorts))
	for exposed := range config.ExposedPorts {
		number, protocol, _ := strings.Cut(exposed, "/")
		if protocol != "" && protocol != "tcp" {
			continue
		}

		if parsed, err := strconv.ParseUint(number, 10, 16); err == nil && parsed > 0 {
			ports = append(ports, uint16(parsed))
		}
	}

	slices.Sort(ports)

	return vm.ImageConfig{
		Entrypoint:   config.Entrypoint,
		Cmd:          config.Cmd,
		Env:          config.Env,
		WorkingDir:   config.WorkingDir,
		User:         config.User,
		ExposedPorts: slices.Compact(ports),
	}
}

// dirName is what an image's directory is called: its digest, with nothing in
// it a path would read differently.
func dirName(digest string) string {
	return strings.Replace(digest, ":", "-", 1)
}

// digestOf is the digest an image's directory is called after, if it is one.
func digestOf(dir string) (string, bool) {
	hash, err := v1.NewHash(strings.Replace(dir, "-", ":", 1))
	if err != nil {
		return "", false
	}

	return hash.String(), true
}

func readJSON(path string, value any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, value)
}

func writeJSON(path string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return writeFile(path, encoded)
}

// writeFile writes a file whole, so that it is never read half written:
// everybody's to read, as everything an image is made of is.
func writeFile(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())

	if _, err := temporary.Write(content); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporary.Name(), path)
}
