// Package output keeps what VMs' tasks wrote, as vmhost's own copy of it: one
// JSON line each, in output.log in the VM's own directory (layout.Output), in
// the order the lines are numbered.
//
// The agent inside a machine numbers its task's lines and keeps them in a ring
// of its own, which a machine that boots again starts afresh; vmhost reads the
// ring as it fills and keeps every line here, numbered after everything the VM
// wrote before. So a VM's output survives its machine, its being started again
// and vmhost's own restarts, and whoever reads it reads a file rather than the
// guest. A line read twice — once as it came, once more when the task ended,
// or again by the vmhost that took over — is kept once, since it carries the
// same number both times.
//
// What is kept is bounded: once a file grows past half of the size a VM may
// keep, it is moved aside and a new one begun, and the one moved aside before
// is let go. The control plane keeps a task's whole log; this only has to hold
// what is still on its way there. Ported from PR #101's runner.
package output

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

const (
	// rotatedSuffix names the file a VM's output is moved aside to.
	rotatedSuffix = ".1"

	// minFileSize is the least a file is let grow to before it is moved
	// aside, however small the size a VM may keep: below it, a single long
	// line would move it aside every time.
	minFileSize = 64 << 10

	// maxLine bounds one line as it is read back: a line is capped at 64 KiB
	// inside the guest, and escaping it can grow it.
	maxLine = 512 << 10
)

// Store keeps VMs' output under the data directory.
type Store struct {
	dataDir string

	// fileSize is how large one file grows before it is moved aside: half
	// of what a VM may keep, since the one moved aside is kept too.
	fileSize int64
}

var _ vm.LogStore = (*Store)(nil)

// NewStore keeps VMs' output under dataDir, at most maxSize bytes of it for
// each VM.
func NewStore(dataDir string, maxSize uint64) *Store {
	return &Store{
		dataDir:  dataDir,
		fileSize: max(minFileSize, int64(min(maxSize, 1<<62))/2),
	}
}

// Writer opens a VM's output for more lines to be kept in it.
func (s *Store) Writer(id string) (vm.LogWriter, error) {
	if !vm.IsID(id) {
		return nil, fmt.Errorf("%w: %q cannot name a vm", vm.ErrInvalid, id)
	}

	return open(layout.Output(s.dataDir, id), s.fileSize)
}

// Reader reads a VM's output from its beginning, as it grows.
func (s *Store) Reader(id string) vm.LogReader {
	// nothing is ever kept under what cannot name a vm: a reader of it
	// reads a file that is not there.
	if !vm.IsID(id) {
		return &follower{path: filepath.Join(s.dataDir, layout.VMsDir, "invalid", layout.OutputName)}
	}

	return &follower{path: layout.Output(s.dataDir, id)}
}

// writer keeps more of one VM's output.
type writer struct {
	path  string
	limit int64

	lock sync.Mutex
	file *os.File
	size int64

	// last is the number of the last line kept.
	last uint64

	// changed is closed, and replaced, whenever a line is kept.
	changed chan struct{}
}

// open opens a VM's output for more to be kept in it, knowing what it kept
// already.
func open(path string, limit int64) (*writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	last, err := lastLine(path)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()

		return nil, err
	}

	return &writer{
		path:    path,
		limit:   limit,
		file:    file,
		size:    info.Size(),
		last:    last,
		changed: make(chan struct{}),
	}, nil
}

// Add keeps one line. A line kept already is not kept again; a gap in the
// numbers is lines the agent let go of before anybody read them, and says so.
func (w *writer) Add(line vm.LogLine) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	if w.file == nil {
		return os.ErrClosed
	}

	if line.Seq <= w.last {
		return nil
	}

	if w.last > 0 && line.Seq > w.last+1 {
		lost := vm.LogLine{
			Seq:     line.Seq - 1,
			Stream:  guest.StreamStderr,
			At:      line.At,
			Content: fmt.Sprintf("[vmhost] %d lines were lost while nobody was reading them", line.Seq-w.last-1),
		}

		if err := w.write(lost); err != nil {
			return err
		}
	}

	if err := w.write(line); err != nil {
		return err
	}

	close(w.changed)
	w.changed = make(chan struct{})

	return nil
}

// write appends one line, moving the file aside first if it would grow past
// its limit. The lock is held.
func (w *writer) write(line vm.LogLine) error {
	encoded, err := json.Marshal(line)
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	if w.size > 0 && w.size+int64(len(encoded)) > w.limit {
		if err := w.rotate(); err != nil {
			return err
		}
	}

	written, err := w.file.Write(encoded)
	w.size += int64(written)

	if err != nil {
		return err
	}

	w.last = line.Seq

	return nil
}

// rotate moves what has been kept aside, letting go of what was moved aside
// before it. The lock is held.
func (w *writer) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}

	if err := os.Rename(w.path, w.path+rotatedSuffix); err != nil {
		return err
	}

	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		w.file = nil

		return err
	}

	w.file, w.size = file, 0

	return nil
}

// Last is the number of the last line kept.
func (w *writer) Last() uint64 {
	w.lock.Lock()
	defer w.lock.Unlock()

	return w.last
}

// Changed is closed once another line is kept.
func (w *writer) Changed() <-chan struct{} {
	w.lock.Lock()
	defer w.lock.Unlock()

	return w.changed
}

// Close lets go of the file. Closing twice is closing once.
func (w *writer) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()

	if w.file == nil {
		return nil
	}

	err := w.file.Close()
	w.file = nil

	return err
}

// lastLine is the number of the last line kept at path, or moved aside from
// it.
func lastLine(path string) (uint64, error) {
	var last uint64

	for _, candidate := range []string{path + rotatedSuffix, path} {
		_, err := readFrom(candidate, 0, func(line vm.LogLine) error {
			last = max(last, line.Seq)

			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
	}

	return last, nil
}

// readFrom reads the whole lines kept in the file at path from offset on, and
// says how far it read. A line still being written, without its newline yet,
// is left for the next read.
func readFrom(path string, offset int64, emit func(vm.LogLine) error) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}

	reader := bufio.NewReaderSize(file, 64<<10)

	for {
		line, err := reader.ReadSlice('\n')

		if errors.Is(err, bufio.ErrBufferFull) {
			// a line longer than the buffer is read whole the slow way.
			rest, restErr := reader.ReadBytes('\n')
			line = append(append([]byte(nil), line...), rest...)
			err = restErr
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return offset, nil
			}

			return offset, err
		}

		offset += int64(len(line))

		if len(line) > maxLine {
			continue
		}

		var kept vm.LogLine
		if err := json.Unmarshal(bytes.TrimSpace(line), &kept); err != nil {
			continue
		}

		if err := emit(kept); err != nil {
			return offset, err
		}
	}
}

// follower reads a VM's output as it grows, across the file being moved
// aside.
type follower struct {
	path string

	// started says whether what was moved aside before the follower began
	// has been read yet.
	started bool

	info   os.FileInfo
	offset int64
}

// Next reads whatever has been kept since the last time.
func (f *follower) Next(emit func(vm.LogLine) error) error {
	if !f.started {
		f.started = true

		if _, err := readFrom(f.path+rotatedSuffix, 0, emit); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	current, err := os.Stat(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	// the file read so far has been moved aside: whatever was added to it
	// before it was is read there, and the new one from its start.
	if f.info != nil && !os.SameFile(f.info, current) {
		if _, err := readFrom(f.path+rotatedSuffix, f.offset, emit); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		f.offset = 0
	}

	f.info = current

	offset, err := readFrom(f.path, f.offset, emit)
	f.offset = offset

	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}
