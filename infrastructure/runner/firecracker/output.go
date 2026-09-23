package firecracker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

const (
	// outputLimit is how large a machine's output grows before it is rotated
	// once; what is older than the one rotated away is let go. The control
	// plane keeps a task's whole log, so this only has to hold what is still
	// on its way there.
	outputLimit = 16 << 20

	// maxOutputLine bounds one line of output as it is kept: a line is capped
	// at 64 KiB inside the guest, and escaping it can grow it.
	maxOutputLine = 512 << 10
)

// output is what a machine's task wrote, as the orchestrator keeps it: one JSON
// line each, in the order the agent numbered them. It is what a task's logs
// are read from, while it runs and after it ends.
type output struct {
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

// openOutput opens a machine's output for more to be kept in it.
func openOutput(path string, limit int64) (*output, error) {
	last, err := lastLine(path)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()

		return nil, err
	}

	return &output{
		path:    path,
		limit:   limit,
		file:    file,
		size:    info.Size(),
		last:    last,
		changed: make(chan struct{}),
	}, nil
}

// add keeps one line. A line kept already is not kept again, which is what
// makes reading the agent twice harmless; a gap in the numbers is lines the
// agent let go of before anybody read them, and says so.
func (o *output) add(line guest.LogLine) error {
	o.lock.Lock()
	defer o.lock.Unlock()

	if line.Seq <= o.last {
		return nil
	}

	if o.last > 0 && line.Seq > o.last+1 {
		lost := guest.LogLine{
			Seq:     line.Seq - 1,
			Stream:  guest.StreamStderr,
			At:      line.At,
			Content: fmt.Sprintf("[runner] %d lines were lost while nobody was reading them", line.Seq-o.last-1),
		}

		if err := o.write(lost); err != nil {
			return err
		}
	}

	if err := o.write(line); err != nil {
		return err
	}

	close(o.changed)
	o.changed = make(chan struct{})

	return nil
}

func (o *output) write(line guest.LogLine) error {
	encoded, err := json.Marshal(line)
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	if o.size+int64(len(encoded)) > o.limit {
		if err := o.rotate(); err != nil {
			return err
		}
	}

	written, err := o.file.Write(encoded)
	o.size += int64(written)
	o.last = line.Seq

	return err
}

// rotate moves what has been kept aside, letting go of what was moved aside
// before it.
func (o *output) rotate() error {
	if err := o.file.Close(); err != nil {
		return err
	}

	if err := os.Rename(o.path, o.path+".1"); err != nil {
		return err
	}

	file, err := os.OpenFile(o.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	o.file, o.size = file, 0

	return nil
}

// lastSeq is the number of the last line kept.
func (o *output) lastSeq() uint64 {
	o.lock.Lock()
	defer o.lock.Unlock()

	return o.last
}

// wait is closed once another line is kept.
func (o *output) wait() <-chan struct{} {
	o.lock.Lock()
	defer o.lock.Unlock()

	return o.changed
}

func (o *output) close() error {
	o.lock.Lock()
	defer o.lock.Unlock()

	return o.file.Close()
}

// lastLine is the number of the last line kept at path.
func lastLine(path string) (uint64, error) {
	var last uint64

	for _, candidate := range []string{path + ".1", path} {
		err := scanOutput(candidate, 0, func(line guest.LogLine) error {
			last = max(last, line.Seq)

			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
	}

	return last, nil
}

// scanOutput reads the whole lines kept in the file at path from offset on,
// and says how far it read. A line still being written is left for the next
// read.
func scanOutput(path string, offset int64, emit func(guest.LogLine) error) error {
	_, err := readOutput(path, offset, emit)

	return err
}

func readOutput(path string, offset int64, emit func(guest.LogLine) error) (int64, error) {
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

		if len(line) > maxOutputLine {
			continue
		}

		var kept guest.LogLine
		if err := json.Unmarshal(bytes.TrimSpace(line), &kept); err != nil {
			continue
		}

		if err := emit(kept); err != nil {
			return offset, err
		}
	}
}

// follower reads a machine's output as it grows, across the rotation that
// moves it aside.
type follower struct {
	path string

	// started says whether what was rotated aside before the follower began
	// has been read yet.
	started bool

	info   os.FileInfo
	offset int64
}

// next reads whatever has been kept since the last time.
func (f *follower) next(emit func(guest.LogLine) error) error {
	if !f.started {
		f.started = true

		if err := scanOutput(f.path+".1", 0, emit); err != nil && !errors.Is(err, os.ErrNotExist) {
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
		if _, err := readOutput(f.path+".1", f.offset, emit); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		f.offset = 0
	}

	f.info = current

	offset, err := readOutput(f.path, f.offset, emit)
	f.offset = offset

	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

// pollInterval is how often a follower with nothing to wait on looks again.
const pollInterval = time.Second
