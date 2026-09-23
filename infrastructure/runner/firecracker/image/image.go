// Package image turns the images tasks name into roots microVMs boot.
//
// A task names an OCI image the way it would for a container: nginx:alpine.
// The image is pulled for the host's platform, its layers are laid over each
// other the way a container runtime would lay them, and the result is written
// into an ext4 filesystem of its own. That filesystem is only ever read: every
// machine running the image shares it, and what a machine changes goes to a
// scratch disk of its own.
//
// An image is kept by its digest, so a tag that moves is a new image rather
// than a changed one, and one digest is built once however many ask for it at
// the same time — orchestrators on the same host included.
package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"

	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

const (
	rootName   = "rootfs.ext4"
	configName = "config.json"
	refsDir    = "refs"

	// what an image's filesystem is sized as, beyond what it holds: ext4's
	// own tables, and room to spare for what does not pack as tightly as it
	// does in a tarball.
	sizeOverhead = 32 << 20
	sizeMargin   = 1.25
	sizeAlign    = 4 << 20
	minimumSize  = 64 << 20

	// bytesPerInode gives an image one inode for every 4 KiB it holds, which
	// is enough for one full of small files.
	bytesPerInode = "4096"
)

// Config is what an image says about how it is run.
type Config struct {
	Entrypoint   []string `json:"entrypoint,omitempty"`
	Cmd          []string `json:"cmd,omitempty"`
	Env          []string `json:"env,omitempty"`
	WorkingDir   string   `json:"working_dir,omitempty"`
	User         string   `json:"user,omitempty"`
	ExposedPorts []uint16 `json:"exposed_ports,omitempty"`
}

// Image is an image made into a root a machine can boot.
type Image struct {
	Digest string
	Root   string
	Config Config
}

// Store keeps images' roots.
type Store struct {
	dir    string
	uid    int
	gid    int
	logger *slog.Logger
	tracer oteltrace.Tracer

	// builds makes one build of a digest serve everybody asking for it at
	// once in this process; a lock file does the same across processes.
	builds singleflight.Group
}

// NewStore keeps images' roots in dir, made for uid and gid to read.
func NewStore(dir string, uid int, gid int, logger *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, refsDir), 0o755); err != nil {
		return nil, err
	}

	if _, err := exec.LookPath("mke2fs"); err != nil {
		return nil, fmt.Errorf("images cannot be made into roots without mke2fs: %w", err)
	}

	return &Store{dir: dir, uid: uid, gid: gid, logger: logger, tracer: otel.Tracer("firecracker")}, nil
}

// Ensure makes sure an image is here, pulling and building it if it is not.
//
// An image already built under the reference it was asked by is taken as it
// is, without asking its registry again: the same as a container runtime,
// which does not pull an image it already holds.
func (s *Store) Ensure(ctx context.Context, reference string) (Image, error) {
	ctx, span := s.tracer.Start(ctx, "firecracker.image.ensure",
		oteltrace.WithAttributes(attribute.String("image", reference)),
	)
	defer span.End()

	if digest, found := s.known(reference); found {
		if built, err := s.load(digest); err == nil {
			return built, nil
		}
	}

	ref, err := name.ParseReference(reference)
	if err != nil {
		return Image{}, trace.RecordError(span, fmt.Errorf("%q is not an image: %w", reference, err))
	}

	pulled, err := remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithPlatform(v1.Platform{OS: "linux", Architecture: goruntime.GOARCH}),
	)
	if err != nil {
		return Image{}, trace.RecordError(span, fmt.Errorf("failed to pull %s: %w", reference, err))
	}

	digest, err := pulled.Digest()
	if err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	built, err, _ := s.builds.Do(digest.String(), func() (any, error) {
		return s.build(ctx, reference, digest, pulled)
	})
	if err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	if err := s.remember(reference, digest.String()); err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	return built.(Image), nil
}

// build makes a pulled image into a root, unless another process already has.
func (s *Store) build(ctx context.Context, reference string, digest v1.Hash, pulled v1.Image) (Image, error) {
	ctx, span := s.tracer.Start(ctx, "firecracker.image.build",
		oteltrace.WithAttributes(attribute.String("image", reference), attribute.String("digest", digest.String())),
	)
	defer span.End()

	unlock, err := lock(filepath.Join(s.dir, "."+dirName(digest.String())+".lock"))
	if err != nil {
		return Image{}, trace.RecordError(span, err)
	}
	defer unlock()

	if built, err := s.load(digest.String()); err == nil {
		return built, nil
	}

	file, err := pulled.ConfigFile()
	if err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	config := configOf(file.Config)

	s.logger.Info("image does not exist, start pulling", "image", reference, "digest", digest.String())

	work, err := os.MkdirTemp(s.dir, ".build-")
	if err != nil {
		return Image{}, trace.RecordError(span, err)
	}
	defer os.RemoveAll(work)

	// a temporary directory is made for its maker alone; this one is for
	// every machine that boots the image.
	if err := os.Chmod(work, 0o755); err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	tarball := filepath.Join(work, "rootfs.tar")

	size, err := flatten(pulled, tarball)
	if err != nil {
		return Image{}, trace.RecordError(span, fmt.Errorf("failed to lay %s's layers out: %w", reference, err))
	}

	root := filepath.Join(work, rootName)
	if err := makeFilesystem(ctx, root, tarball, filesystemSize(size)); err != nil {
		return Image{}, trace.RecordError(span, fmt.Errorf("failed to make %s into a root: %w", reference, err))
	}

	if err := os.Remove(tarball); err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	if err := os.WriteFile(filepath.Join(work, configName), encoded, 0o644); err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	for _, path := range []string{work, root, filepath.Join(work, configName)} {
		if err := s.own(path); err != nil {
			return Image{}, trace.RecordError(span, err)
		}
	}

	// moved into place whole, so a root is either there or it is not.
	if err := os.Rename(work, filepath.Join(s.dir, dirName(digest.String()))); err != nil {
		return Image{}, trace.RecordError(span, err)
	}

	s.logger.Info("image pulled", "image", reference, "digest", digest.String())

	return s.load(digest.String())
}

// load reads back an image that has been built.
func (s *Store) load(digest string) (Image, error) {
	dir := filepath.Join(s.dir, dirName(digest))

	encoded, err := os.ReadFile(filepath.Join(dir, configName))
	if err != nil {
		return Image{}, err
	}

	var config Config
	if err := json.Unmarshal(encoded, &config); err != nil {
		return Image{}, err
	}

	root := filepath.Join(dir, rootName)
	if _, err := os.Stat(root); err != nil {
		return Image{}, err
	}

	return Image{Digest: digest, Root: root, Config: config}, nil
}

// known is the digest an image was last built under by reference.
func (s *Store) known(reference string) (string, bool) {
	digest, err := os.ReadFile(s.refPath(reference))
	if err != nil {
		return "", false
	}

	return strings.TrimSpace(string(digest)), true
}

func (s *Store) remember(reference string, digest string) error {
	temporary, err := os.CreateTemp(filepath.Join(s.dir, refsDir), ".ref-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())

	if _, err := temporary.WriteString(digest); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporary.Name(), s.refPath(reference))
}

func (s *Store) refPath(reference string) string {
	sum := sha256.Sum256([]byte(reference))

	return filepath.Join(s.dir, refsDir, hex.EncodeToString(sum[:]))
}

// own makes a file whoever machines run as, when this process may give it
// away: one that runs as them already makes its files theirs.
func (s *Store) own(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}

	return os.Chown(path, s.uid, s.gid)
}

// flatten writes an image's layers, laid over each other, as one tarball, and
// says how large it came out.
func flatten(img v1.Image, path string) (int64, error) {
	file, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	layers := mutate.Extract(img)
	defer layers.Close()

	written, err := io.Copy(file, layers)
	if err != nil {
		return 0, err
	}

	return written, file.Close()
}

// filesystemSize is how large a filesystem holding a tarball of size bytes is
// made.
func filesystemSize(size int64) int64 {
	target := int64(float64(size)*sizeMargin) + sizeOverhead
	target = (target + sizeAlign - 1) / sizeAlign * sizeAlign

	return max(target, minimumSize)
}

// makeFilesystem makes an ext4 filesystem at path holding what the tarball
// holds, as it holds it: whose each file is survives, which extracting the
// tarball first would not, unless whoever did it were root. Nothing writes to
// it once it is made, so it keeps no journal.
func makeFilesystem(ctx context.Context, path string, tarball string, size int64) error {
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return err
	}

	if err := os.Truncate(path, size); err != nil {
		return err
	}

	command := exec.CommandContext(ctx, "mke2fs",
		"-q", "-F",
		"-t", "ext4",
		"-O", "^has_journal",
		"-E", "root_owner=0:0",
		"-i", bytesPerInode,
		"-L", "image",
		"-d", tarball,
		path,
	)

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// configOf reads what an image says about how it is run.
func configOf(config v1.Config) Config {
	ports := make([]uint16, 0, len(config.ExposedPorts))
	for exposed := range config.ExposedPorts {
		number, protocol, _ := strings.Cut(exposed, "/")
		if protocol != "" && protocol != "tcp" {
			continue
		}

		if parsed, err := strconv.ParseUint(number, 10, 16); err == nil {
			ports = append(ports, uint16(parsed))
		}
	}

	slices.Sort(ports)

	return Config{
		Entrypoint:   config.Entrypoint,
		Cmd:          config.Cmd,
		Env:          config.Env,
		WorkingDir:   config.WorkingDir,
		User:         config.User,
		ExposedPorts: ports,
	}
}

// dirName is what an image's directory is called: its digest, with nothing in
// it a path would read differently.
func dirName(digest string) string {
	return strings.ReplaceAll(digest, ":", "-")
}
