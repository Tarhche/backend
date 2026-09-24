// Package image turns the images tasks name into roots microVMs boot.
//
// A task names an OCI image the way it would for a container: nginx:alpine.
// The image is pulled for the host's platform, its layers are laid over each
// other the way a container runtime would lay them, and the result is written
// into a squashfs filesystem of its own. That filesystem is only ever read, and
// is made to be: it is compressed, and every machine running the image shares
// it, while what a machine changes goes to a scratch disk of its own.
//
// An image is kept by its digest, so a tag that moves is a new image rather
// than a changed one, and one digest is built once however many ask for it at
// the same time — orchestrators on the same host included.
package image

import (
	"archive/tar"
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
	pathpkg "path"
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
	rootName   = "rootfs.squashfs"
	configName = "config.json"
	refsDir    = "refs"

	// compressionLevel keeps making a root quick: zstd reads back as fast
	// at any level, and its higher ones take far longer to write.
	compressionLevel = "3"
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

	if _, err := exec.LookPath("sqfstar"); err != nil {
		return nil, fmt.Errorf("images cannot be made into roots without sqfstar: %w", err)
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

	root := filepath.Join(work, rootName)
	if err := makeFilesystem(ctx, pulled, root); err != nil {
		return Image{}, trace.RecordError(span, fmt.Errorf("failed to make %s into a root: %w", reference, err))
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

// makeFilesystem writes an image's layers, laid over each other, into a
// squashfs filesystem at path. The layers travel as a tarball straight into
// sqfstar, which keeps whose each file is as the tarball says it: extracting
// them first would lose that, unless whoever did it were root.
//
// The root is root's, and so is any directory the tarball only implies, as
// they would be in a container: sqfstar would otherwise give them to whoever
// runs it.
func makeFilesystem(ctx context.Context, img v1.Image, path string) error {
	layers := mutate.Extract(img)
	defer layers.Close()

	reader, writer := io.Pipe()

	go func() {
		writer.CloseWithError(normalize(layers, writer))
	}()

	command := exec.CommandContext(ctx, "sqfstar",
		"-quiet",
		"-no-progress",
		"-comp", "zstd",
		"-Xcompression-level", compressionLevel,
		"-root-uid", "0",
		"-root-gid", "0",
		"-root-mode", "0755",
		"-default-uid", "0",
		"-default-gid", "0",
		"-default-mode", "0755",
		path,
	)
	command.Stdin = reader

	output, err := command.CombinedOutput()

	// whatever sqfstar left unread is let go of, so normalize is not left
	// waiting to write it.
	reader.CloseWithError(io.ErrClosedPipe)

	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// normalize copies a tarball, naming every entry the way sqfstar takes it:
// relative, and with nothing in it that is "." or "..". Layers are written by
// whatever built the image, and plenty of them name their entries "./bin/sh"
// or have one for "./" itself. The root is left out, since sqfstar makes it.
func normalize(in io.Reader, out io.Writer) error {
	source := tar.NewReader(in)
	target := tar.NewWriter(out)

	for {
		header, err := source.Next()
		if errors.Is(err, io.EOF) {
			return target.Close()
		}

		if err != nil {
			return err
		}

		header.Name = cleanEntry(header.Name)
		if len(header.Name) == 0 {
			continue
		}

		// a hard link names another entry of the tarball, which is named the
		// same way; a symlink's target is what the link says, and is left as
		// it is.
		if header.Typeflag == tar.TypeLink {
			header.Linkname = cleanEntry(header.Linkname)
		}

		if err := target.WriteHeader(header); err != nil {
			return err
		}

		if _, err := io.Copy(target, source); err != nil {
			return err
		}
	}
}

// cleanEntry is an entry's name relative to the root, or empty for the root
// itself. It is cleaned as though the root were the whole filesystem, so a
// name climbing out of the root lands inside it instead, as it would inside a
// container.
func cleanEntry(name string) string {
	return strings.TrimPrefix(pathpkg.Clean("/"+name), "/")
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
