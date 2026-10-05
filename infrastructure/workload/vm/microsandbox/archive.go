package microsandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Name and Version are this engine's, as Info and the archives it writes say.
//
// Version is the microsandbox release the SDK in go.mod belongs to, and the
// image the vmhost runs on is that release's too: an SDK and a runtime of
// different releases break the database they share under MSB_HOME. An archive
// is read back only by the release that wrote it, since nothing promises that
// another release loads it.
const (
	Name    = "microsandbox"
	Version = "0.7.6"
)

// archiveEngine is what an archive this engine writes says wrote it.
func archiveEngine() string {
	return Name + "/" + Version
}

// archiveHeader is the line an archive starts with: what wrote it and what it
// holds. The bytes microsandbox saved follow it.
//
// It is JSON with the engine first, the way another engine's archive says
// whose it is, so that one is told apart from the other after a few bytes and
// before anything of it is loaded: another engine's archive may carry a whole
// disk on its first line.
type archiveHeader struct {
	Engine string  `json:"engine"`
	Kind   vm.Kind `json:"kind"`
	Image  string  `json:"image"`

	// Disk is the disk, in bytes, the instance was given when it was archived.
	Disk uint64 `json:"disk"`
}

// errNotAnArchive is a stream that does not start the way an archive of the
// workload's engines does.
var errNotAnArchive = errors.New("it does not start with what wrote it")

// writeHeader writes the line an archive starts with, and says how many bytes
// it took.
func writeHeader(w io.Writer, header archiveHeader) (int64, error) {
	line, err := json.Marshal(header)
	if err != nil {
		return 0, err
	}

	n, err := w.Write(append(line, '\n'))

	return int64(n), err
}

// readHeader reads the line an archive starts with and hands back the rest of
// the stream, which is what microsandbox saved.
//
// An archive another engine wrote, or anything that is not an archive at all,
// is refused with vm.ErrEngineMismatch: either way it is not one this engine
// can load.
func readHeader(r io.Reader) (archiveHeader, io.Reader, error) {
	decoder := json.NewDecoder(r)

	engine, err := leadingEngine(decoder)
	if err != nil {
		return archiveHeader{}, nil, fmt.Errorf("%w: %w", vm.ErrEngineMismatch, err)
	}

	if engine != archiveEngine() {
		return archiveHeader{}, nil, fmt.Errorf("%w: written by %q, and this is %q", vm.ErrEngineMismatch, engine, archiveEngine())
	}

	header := archiveHeader{Engine: engine}

	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return archiveHeader{}, nil, fmt.Errorf("the archive's header cannot be read: %w", err)
		}

		switch key {
		case "kind":
			err = decoder.Decode(&header.Kind)
		case "image":
			err = decoder.Decode(&header.Image)
		case "disk":
			err = decoder.Decode(&header.Disk)
		default:
			var skipped json.RawMessage
			err = decoder.Decode(&skipped)
		}

		if err != nil {
			return archiveHeader{}, nil, fmt.Errorf("the archive's header cannot be read: %w", err)
		}
	}

	if _, err := decoder.Token(); err != nil {
		return archiveHeader{}, nil, fmt.Errorf("the archive's header cannot be read: %w", err)
	}

	// the decoder has read ahead of the header: what it holds is the start of
	// the rest, after the newline that ends the header.
	rest := io.MultiReader(decoder.Buffered(), r)

	var newline [1]byte
	if _, err := io.ReadFull(rest, newline[:]); err != nil || newline[0] != '\n' {
		return archiveHeader{}, nil, errors.New("the archive's header does not end its line")
	}

	return header, rest, nil
}

// leadingEngine reads the opening of a header up to the engine that wrote it.
func leadingEngine(decoder *json.Decoder) (string, error) {
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return "", errNotAnArchive
	}

	if token, err := decoder.Token(); err != nil || token != "engine" {
		return "", errNotAnArchive
	}

	token, err := decoder.Token()
	if err != nil {
		return "", errNotAnArchive
	}

	engine, ok := token.(string)
	if !ok {
		return "", errNotAnArchive
	}

	return engine, nil
}
