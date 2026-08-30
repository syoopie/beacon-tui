package logtail

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// Tailer follows an append-only log file. It reopens the file when it is
// truncated or replaced, because nothing rotates these logs but an operator with
// a shell, and beacon should keep following after they do.
type Tailer struct {
	path    string
	f       *os.File    // nil when the file is not open
	info    os.FileInfo // identity of f as opened, for os.SameFile
	offset  int64       // bytes consumed from f
	partial []byte      // trailing bytes not yet terminated by a newline
}

func NewTailer(path string) *Tailer { return &Tailer{path: path} }

// Read returns the complete lines appended since the previous call. A file that
// does not exist yet is not an error; it returns nil, nil.
func (t *Tailer) Read() ([]string, error) {
	info, err := os.Stat(t.path)
	if os.IsNotExist(err) {
		t.reset()
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", t.path, err)
	}
	if t.f != nil && (!os.SameFile(t.info, info) || info.Size() < t.offset) {
		t.reset()
	}
	if t.f == nil {
		if err := t.open(); err != nil {
			return nil, err
		}
	}

	if _, err := t.f.Seek(t.offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek %s: %w", t.path, err)
	}
	chunk, err := io.ReadAll(t.f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.path, err)
	}
	t.offset += int64(len(chunk))
	if len(chunk) == 0 {
		return nil, nil
	}

	buf := append(t.partial, chunk...)
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		t.partial = buf
		return nil, nil
	}
	t.partial = append([]byte(nil), buf[end+1:]...)
	return strings.Split(string(buf[:end]), "\n"), nil
}

func (t *Tailer) open() error {
	f, err := os.Open(t.path)
	if err != nil {
		return fmt.Errorf("open %s: %w", t.path, err)
	}
	// Stat the handle rather than the path, so info identifies the file actually
	// being read even if the path is replaced in between.
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat %s: %w", t.path, err)
	}
	t.f, t.info = f, info
	// A carried-over offset past the end of the file we just opened would seek
	// past EOF and stall for good. Reaching here with one means the size and
	// identity checks in Read were skipped, because they only run for an already
	// open handle.
	if info.Size() < t.offset {
		t.offset, t.partial = 0, nil
	}
	return nil
}

func (t *Tailer) reset() {
	_ = t.Close()
	t.offset = 0
	t.partial = nil
}

func (t *Tailer) Close() error {
	if t.f == nil {
		return nil
	}
	err := t.f.Close()
	t.f, t.info = nil, nil
	return err
}
