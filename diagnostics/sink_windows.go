//go:build windows

// Windows admits disk files only. Console and pipe handles are synchronous and
// cannot promise a cancellable, joinable write, so they remain refused.
package diagnostics

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// A disk file has no peer that can stall a write; the private duplicate
// shares the caller's file position and is closed only by the exporter.
type fileSink struct{ handle windows.Handle }

// Admission holds the original File's handle reference while duplicating it.
func newDescriptorSink(file *os.File) (ownedSink, error) {
	if file == nil {
		return nil, errors.New("diagnostic file is absent")
	}
	connection, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	var result ownedSink
	var admissionErr error
	err = connection.Control(func(raw uintptr) {
		original := windows.Handle(raw)
		var kind uint32
		if kind, admissionErr = windows.GetFileType(original); admissionErr != nil {
			return
		}
		if kind != windows.FILE_TYPE_DISK {
			admissionErr = errors.New("diagnostic handle cannot promise a cancellable write")
			return
		}
		process := windows.CurrentProcess()
		var owned windows.Handle
		if admissionErr = windows.DuplicateHandle(process, original, process, &owned, 0, false, windows.DUPLICATE_SAME_ACCESS); admissionErr == nil {
			result = &fileSink{handle: owned}
		}
	})
	if err = errors.Join(err, admissionErr); err != nil {
		if result != nil {
			err = errors.Join(err, result.Close())
		}
		return nil, err
	}
	return result, nil
}

// Records are at most MaximumRecordBytes; a short write resumes at its suffix.
func (self *fileSink) WriteContext(ctx context.Context, raw []byte) (int, error) {
	written := 0
	for written < len(raw) {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		var count uint32
		err := windows.WriteFile(self.handle, raw[written:], &count, nil)
		written += int(count)
		if err != nil {
			return written, err
		}
		if count == 0 {
			return written, errors.New("diagnostic file accepted no bytes")
		}
	}
	return written, nil
}

// The exporter joins its write before releasing this owned handle only.
func (self *fileSink) Close() error { return windows.CloseHandle(self.handle) }
