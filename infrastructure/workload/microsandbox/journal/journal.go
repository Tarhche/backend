// Package journal keeps what each run's main process wrote, on disk, for the
// workload-microsandbox service.
//
// Microsandbox keeps a log of its own, but only of the first command of each
// boot, and stamps it to the millisecond. A run's main process is a new command
// at every boot, and a reader resuming from a stamp needs stamps that tell
// lines apart. So the service keeps its own: one journal for each run, which
// outlives the run's VM, its restarts, and the service, and goes when the run
// is deleted.
//
// # On disk
//
// A journal is newline-delimited JSON, one api.LogLine on each line, as the
// service streams it. It is kept in numbered segments, <run>.<n>.ndjson, each
// up to half the journal's capacity: when the newest is full the next is
// begun, and the one before it goes. So a journal holds the newest lines its
// run wrote, between half its capacity and all of it, and a reader following
// it moves from one segment to the next as it reaches each one's end.
package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	// DefaultCapacity is how much of a run's output its journal keeps.
	DefaultCapacity = 32 << 20

	extension = ".ndjson"

	fileMode      = 0o600
	directoryMode = 0o700

	// tailSize is how much of a segment's end is read to find its last
	// line: more than the longest line can take, which is a 64 KiB content
	// whose every byte JSON escapes in six.
	tailSize = 512 << 10
)

// validID is what a run's ID may be. IDs are the service's own, but they name
// files, so one that could climb out of the directory is refused all the same.
var validID = regexp.MustCompile(`^[0-9A-Za-z_-]{1,128}$`)

// Journal keeps the journals of every run in one directory. It is safe for
// concurrent use, with one writer for each run at a time, which the
// supervisor's keeper is.
type Journal struct {
	directory   string
	segmentSize int64

	mu sync.Mutex

	// segments are the numbers of each run's segments, oldest first, read
	// from the directory the first time a run's journal is used.
	segments map[string][]int

	// writers are the newest segments, open for appending.
	writers map[string]*writer
}

type writer struct {
	file   *os.File
	number int
	size   int64
}

var _ runs.Journal = &Journal{}

// New keeps journals in directory, which is made when it is not there, each
// holding at most capacity bytes of a run's newest output.
func New(directory string, capacity int64) (*Journal, error) {
	if capacity < 2 {
		return nil, fmt.Errorf("journal: a capacity of %d bytes holds nothing", capacity)
	}

	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return nil, fmt.Errorf("journal: the journals' directory could not be made: %w", err)
	}

	return &Journal{
		directory:   directory,
		segmentSize: capacity / 2,
		segments:    make(map[string][]int),
		writers:     make(map[string]*writer),
	}, nil
}

// Append adds lines to the end of a run's journal, in one write, so a reader
// sees each line whole or not at all.
func (j *Journal) Append(id string, lines []api.LogLine) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("journal: %q is not a run's ID", id)
	}

	if len(lines) == 0 {
		return nil
	}

	var buffer bytes.Buffer

	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)

	for _, line := range lines {
		// the encoder ends each value with the newline that delimits it.
		if err := encoder.Encode(line); err != nil {
			return fmt.Errorf("journal: a line of run %s could not be encoded: %w", id, err)
		}
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	w, err := j.writer(id)
	if err != nil {
		return err
	}

	if w.size > 0 && w.size+int64(buffer.Len()) > j.segmentSize {
		if w, err = j.rotate(id, w); err != nil {
			return err
		}
	}

	n, err := w.file.Write(buffer.Bytes())
	w.size += int64(n)

	if err != nil {
		return fmt.Errorf("journal: run %s's output could not be written: %w", id, err)
	}

	return nil
}

// Reader reads a run's journal from its first line at or after since. A run
// with no journal yet has an empty one, which fills as the run writes.
func (j *Journal) Reader(id string, since time.Time) (runs.JournalReader, error) {
	if !validID.MatchString(id) {
		return nil, fmt.Errorf("journal: %q is not a run's ID", id)
	}

	j.mu.Lock()
	numbers, err := j.numbers(id)
	j.mu.Unlock()

	if err != nil {
		return nil, err
	}

	r := &reader{journal: j, id: id, since: since}

	if len(numbers) == 0 {
		return r, nil
	}

	// the reader starts at the newest segment that begins at or before
	// since: every line of the ones before it is older.
	r.number = numbers[0]

	if !since.IsZero() {
		for _, number := range slices.Backward(numbers) {
			first, found, err := j.firstStamp(id, number)
			if err != nil {
				return nil, err
			}

			if found && !first.After(since) {
				r.number = number

				break
			}
		}
	}

	return r, nil
}

// Last is the stamp of the newest line in a run's journal, and zero when it
// has none.
func (j *Journal) Last(id string) (time.Time, error) {
	if !validID.MatchString(id) {
		return time.Time{}, fmt.Errorf("journal: %q is not a run's ID", id)
	}

	j.mu.Lock()
	numbers, err := j.numbers(id)
	j.mu.Unlock()

	if err != nil {
		return time.Time{}, err
	}

	for _, number := range slices.Backward(numbers) {
		last, found, err := j.lastStamp(id, number)
		if err != nil {
			return time.Time{}, err
		}

		if found {
			return last, nil
		}
	}

	return time.Time{}, nil
}

// Delete removes a run's journal. One that is not there is no error.
func (j *Journal) Delete(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("journal: %q is not a run's ID", id)
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	if w, open := j.writers[id]; open {
		_ = w.file.Close()
		delete(j.writers, id)
	}

	numbers, err := j.numbers(id)
	if err != nil {
		return err
	}

	delete(j.segments, id)

	var errs []error

	for _, number := range numbers {
		if err := os.Remove(j.path(id, number)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("journal: run %s's journal could not be removed: %w", id, err)
	}

	return nil
}

// Close closes every segment open for appending. The journal can still be
// used: a segment is opened again when it is next appended to.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	var errs []error

	for id, w := range j.writers {
		errs = append(errs, w.file.Close())
		delete(j.writers, id)
	}

	return errors.Join(errs...)
}

// writer is the newest segment of a run's journal, open for appending, begun
// if the run has none. Called with mu held.
func (j *Journal) writer(id string) (*writer, error) {
	if w, open := j.writers[id]; open {
		return w, nil
	}

	numbers, err := j.numbers(id)
	if err != nil {
		return nil, err
	}

	number := 1
	if len(numbers) > 0 {
		number = numbers[len(numbers)-1]
	}

	file, err := os.OpenFile(j.path(id, number), os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		return nil, fmt.Errorf("journal: run %s's journal could not be opened: %w", id, err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("journal: run %s's journal could not be opened: %w", id, err)
	}

	if len(numbers) == 0 {
		j.segments[id] = []int{number}
	}

	w := &writer{file: file, number: number, size: info.Size()}
	j.writers[id] = w

	return w, nil
}

// rotate begins a run's next segment, and removes every segment but the one
// that was full, so that the journal holds at most its capacity. Called with
// mu held.
func (j *Journal) rotate(id string, full *writer) (*writer, error) {
	next := full.number + 1

	file, err := os.OpenFile(j.path(id, next), os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		return nil, fmt.Errorf("journal: run %s's journal could not be continued: %w", id, err)
	}

	_ = full.file.Close()

	numbers := j.segments[id]

	kept := []int{full.number, next}
	for _, number := range numbers {
		if number < full.number {
			_ = os.Remove(j.path(id, number))
		}
	}

	j.segments[id] = kept

	w := &writer{file: file, number: next}
	j.writers[id] = w

	return w, nil
}

// numbers are a run's segments, oldest first. Called with mu held.
func (j *Journal) numbers(id string) ([]int, error) {
	if numbers, known := j.segments[id]; known {
		return slices.Clone(numbers), nil
	}

	matches, err := filepath.Glob(filepath.Join(j.directory, id+".*"+extension))
	if err != nil {
		return nil, fmt.Errorf("journal: run %s's journal could not be listed: %w", id, err)
	}

	var numbers []int

	for _, match := range matches {
		name := strings.TrimSuffix(filepath.Base(match), extension)

		number, err := strconv.Atoi(strings.TrimPrefix(name, id+"."))
		if err != nil || number < 1 {
			continue
		}

		numbers = append(numbers, number)
	}

	slices.Sort(numbers)

	if len(numbers) > 0 {
		j.segments[id] = numbers
	}

	return slices.Clone(numbers), nil
}

// after is the oldest segment of a run's journal after number, and false when
// number is the newest.
func (j *Journal) after(id string, number int) (int, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	numbers, err := j.numbers(id)
	if err != nil {
		return 0, false
	}

	for _, n := range numbers {
		if n > number {
			return n, true
		}
	}

	return 0, false
}

// firstStamp is the stamp of a segment's first line.
func (j *Journal) firstStamp(id string, number int) (time.Time, bool, error) {
	file, err := os.Open(j.path(id, number))
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, false, nil
	}

	if err != nil {
		return time.Time{}, false, fmt.Errorf("journal: run %s's journal could not be read: %w", id, err)
	}
	defer file.Close()

	line, err := bufio.NewReaderSize(file, 64<<10).ReadBytes('\n')
	if err != nil {
		// no complete line, or none at all.
		return time.Time{}, false, nil
	}

	var first api.LogLine
	if err := json.Unmarshal(line, &first); err != nil {
		return time.Time{}, false, nil
	}

	return first.At, true, nil
}

// lastStamp is the stamp of a segment's last complete line.
func (j *Journal) lastStamp(id string, number int) (time.Time, bool, error) {
	file, err := os.Open(j.path(id, number))
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, false, nil
	}

	if err != nil {
		return time.Time{}, false, fmt.Errorf("journal: run %s's journal could not be read: %w", id, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return time.Time{}, false, fmt.Errorf("journal: run %s's journal could not be read: %w", id, err)
	}

	offset := max(info.Size()-tailSize, 0)

	tail := make([]byte, info.Size()-offset)
	if _, err := file.ReadAt(tail, offset); err != nil && !errors.Is(err, io.EOF) {
		return time.Time{}, false, fmt.Errorf("journal: run %s's journal could not be read: %w", id, err)
	}

	// the last complete line ends with the last newline; whatever follows it
	// is a line still being written.
	tail = tail[:bytes.LastIndexByte(tail, '\n')+1]

	for len(tail) > 0 {
		body := tail[:len(tail)-1]
		start := bytes.LastIndexByte(body, '\n') + 1

		var last api.LogLine
		if err := json.Unmarshal(body[start:], &last); err == nil {
			return last.At, true, nil
		}

		tail = tail[:start]
	}

	return time.Time{}, false, nil
}

func (j *Journal) path(id string, number int) string {
	return filepath.Join(j.directory, id+"."+strconv.Itoa(number)+extension)
}

// reader reads one run's journal, segment by segment.
type reader struct {
	journal *Journal
	id      string
	since   time.Time

	// number is the segment being read, and zero before the journal has
	// any.
	number int

	file     *os.File
	buffered *bufio.Reader

	// complete is the segment after the one being read, once the one being
	// read is known to have been written to its end, and zero until then.
	complete int

	// partial is the start of a line whose end has not been written yet.
	partial []byte
}

var _ runs.JournalReader = &reader{}

// Next is the next line, or io.EOF at the end of what has been written so far.
// Lines before since are skipped, and so is a line that cannot be read, which
// a journal can only hold if its disk failed it.
func (r *reader) Next() (api.LogLine, error) {
	for {
		if r.file == nil {
			opened, err := r.open()
			if err != nil {
				return api.LogLine{}, err
			}

			if !opened {
				return api.LogLine{}, io.EOF
			}
		}

		chunk, err := r.buffered.ReadBytes('\n')

		if errors.Is(err, io.EOF) {
			r.partial = append(r.partial, chunk...)

			// a segment with a newer one after it is complete: the writer
			// only begins the next once it is done with this one. So it is
			// read to its end once more, since what was written before the
			// next was begun may have landed after the read above, and then
			// left for the next.
			if r.complete > 0 {
				r.moveTo(r.complete)

				continue
			}

			next, newer := r.journal.after(r.id, r.number)
			if !newer {
				return api.LogLine{}, io.EOF
			}

			r.complete = next

			continue
		}

		if err != nil {
			return api.LogLine{}, fmt.Errorf("journal: run %s's journal could not be read: %w", r.id, err)
		}

		r.partial = append(r.partial, chunk...)

		if line, ok := r.take(); ok {
			return line, nil
		}
	}
}

// take reads the line that partial now holds whole, and reports false for one
// before since or one that cannot be read.
func (r *reader) take() (api.LogLine, bool) {
	data := r.partial
	r.partial = nil

	if len(data) == 0 || data[len(data)-1] != '\n' {
		return api.LogLine{}, false
	}

	var line api.LogLine
	if err := json.Unmarshal(data, &line); err != nil {
		return api.LogLine{}, false
	}

	if line.At.Before(r.since) {
		return api.LogLine{}, false
	}

	return line, true
}

// open opens the segment being read, or the oldest one there is if the
// journal had none when the reader was made, and reports false when there is
// nothing to open yet.
func (r *reader) open() (bool, error) {
	if r.number == 0 {
		next, found := r.journal.after(r.id, 0)
		if !found {
			return false, nil
		}

		r.number = next
	}

	file, err := os.Open(r.journal.path(r.id, r.number))
	if errors.Is(err, fs.ErrNotExist) {
		// it went while this was behind it: the oldest that is left
		// follows.
		next, found := r.journal.after(r.id, r.number)
		if !found {
			return false, nil
		}

		r.number = next

		return r.open()
	}

	if err != nil {
		return false, fmt.Errorf("journal: run %s's journal could not be read: %w", r.id, err)
	}

	r.file = file
	r.buffered = bufio.NewReaderSize(file, 64<<10)

	return true, nil
}

func (r *reader) moveTo(number int) {
	_ = r.file.Close()

	r.file = nil
	r.buffered = nil
	r.complete = 0
	r.partial = nil
	r.number = number
}

// Close lets go of the segment being read.
func (r *reader) Close() error {
	if r.file == nil {
		return nil
	}

	err := r.file.Close()
	r.file = nil

	return err
}
