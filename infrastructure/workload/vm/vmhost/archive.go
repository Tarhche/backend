package vmhost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

// errRestoreOver is what the transport is told when it reads a restore's
// archive after the restore has returned.
var errRestoreOver = errors.New("the restore is over")

// Snapshot writes a VM's archive to archive as the vmhost streams it, and
// says what was written once all of it has arrived.
//
// Nothing of the archive is held here beyond what one copy takes. An archive
// cut off on the way, one the vmhost says failed after it began streaming,
// and one that is not as long as the vmhost says it wrote, are all failures:
// what was written to archive by then is not an archive.
func (c *Client) Snapshot(ctx context.Context, id string, archive io.Writer) (vm.Archive, error) {
	ctx, span := c.start(ctx, "Snapshot", id)
	defer span.End()

	if len(id) == 0 {
		return vm.Archive{}, notThere(id)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+wire.PathVM(id, wire.ActionSnapshot), nil)
	if err != nil {
		return vm.Archive{}, err
	}

	response, err := c.do(ctx, request)
	if err != nil {
		return vm.Archive{}, record(span, err)
	}

	// a body that was not read to its end closes the connection, which is
	// what stops the vmhost writing an archive nobody takes.
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return vm.Archive{}, record(span, errorOf(response))
	}

	written, err := receiveArchive(response, archive)

	return written, record(span, err)
}

// receiveArchive copies a snapshot's archive to archive, and reads what the
// vmhost says it wrote off the trailers that follow it.
func receiveArchive(response *http.Response, archive io.Writer) (vm.Archive, error) {
	sink := &sink{writer: archive}

	copied, err := io.Copy(sink, response.Body)
	if err != nil {
		// the caller's writer refused: its error, as it gave it.
		if sink.err != nil {
			return vm.Archive{}, sink.err
		}

		return vm.Archive{}, fmt.Errorf("the archive was cut off after %d bytes: %w", copied, err)
	}

	if failure := response.Trailer.Get(wire.ErrorTrailer); len(failure) > 0 {
		var answered wire.Error
		if err := wire.DecodeHeader(failure, &answered); err != nil || len(answered.Code) == 0 {
			return vm.Archive{}, fmt.Errorf("the archive failed after %d bytes, and why cannot be read", copied)
		}

		return vm.Archive{}, &answered
	}

	said := response.Trailer.Get(wire.ArchiveTrailer)
	if len(said) == 0 {
		return vm.Archive{}, fmt.Errorf("the vmhost did not say what it wrote after %d bytes of archive", copied)
	}

	var written wire.Archive
	if err := wire.DecodeHeader(said, &written); err != nil {
		return vm.Archive{}, fmt.Errorf("what the vmhost says it wrote cannot be read: %w", err)
	}

	if written.Size != copied {
		return vm.Archive{}, fmt.Errorf("the vmhost wrote %d bytes of archive and %d arrived", written.Size, copied)
	}

	return written.ToVM(), nil
}

// Restore replaces the VM spec.ID names, or creates it, from archive, which
// is streamed to the vmhost as it is read.
//
// archive is read until Restore returns and never after, except for a read
// already under way when it returns, which is left to finish: a caller that
// closes its archive once Restore has returned ends that one too.
func (c *Client) Restore(ctx context.Context, spec vm.Spec, archive io.Reader) (vm.Instance, error) {
	ctx, span := c.start(ctx, "Restore", spec.ID)
	defer span.End()

	encoded, err := wire.EncodeHeader(wire.NewSpec(spec))
	if err != nil {
		return vm.Instance{}, err
	}

	body, sent := send(ctx, archive)
	defer sent()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+wire.PathRestore, body)
	if err != nil {
		return vm.Instance{}, err
	}

	// its length is not known until it has been read: it is sent in chunks.
	request.ContentLength = -1
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set(wire.SpecHeader, encoded)

	response, err := c.do(ctx, request)
	if err != nil {
		return vm.Instance{}, record(span, err)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusMultipleChoices {
		return vm.Instance{}, record(span, errorOf(response))
	}

	var instance wire.Instance
	if err := decodeAnswer(response, &instance); err != nil {
		return vm.Instance{}, record(span, err)
	}

	return instance.ToVM(), nil
}

// sink is the caller's writer, keeping what it refused with, so a copy that
// failed says whose failure it was.
type sink struct {
	writer io.Writer
	err    error
}

func (s *sink) Write(p []byte) (int, error) {
	n, err := s.writer.Write(p)
	if err != nil {
		s.err = err
	}

	return n, err
}

// send is the caller's archive as the transport reads it, and what is called
// once the restore is over.
//
// It is read by a goroutine of its own and handed on through a pipe, so that
// giving up on ctx lets go of the transport at once. The transport waits for
// whoever reads a request's body to be done before it says the request
// failed, and a read of an archive that is not arriving — a bucket that went
// quiet — would otherwise hold a restore nobody waits for any more until the
// bucket answered. Closing the pipe once the restore is over stops the
// goroutine reading the archive at its next read.
func send(ctx context.Context, archive io.Reader) (io.Reader, func()) {
	reader, writer := io.Pipe()

	go func() {
		_, err := io.Copy(writer, archive)
		_ = writer.CloseWithError(err)
	}()

	stop := context.AfterFunc(ctx, func() {
		_ = reader.CloseWithError(context.Cause(ctx))
	})

	return reader, func() {
		stop()
		_ = reader.CloseWithError(errRestoreOver)
	}
}
