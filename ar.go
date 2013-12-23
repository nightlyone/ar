// Package ar can read common ar archives. Those are often used in software development tools.
// Even *.deb files are actually a special case of the common ar archive.
// See http://en.wikipedia.org/wiki/Ar_(Unix) for more information on this file format.
package ar

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"syscall"
	"time"
)

const (
	magic     = "!<arch>\n"
	filemagic = "\x60\x0A"
)

type file struct {
	name [16]uint8 // Filename in  ASCII

}

type fileInfo struct {
	name  string
	mode  os.FileMode
	size  int64
	mtime time.Time
}

// IsDir returns always false for ar archive members, because we don't support directories.
func (f *fileInfo) IsDir() bool { return false }

func (f *fileInfo) ModTime() time.Time { return f.mtime }
func (f *fileInfo) Mode() os.FileMode  { return f.mode }
func (f *fileInfo) Name() string       { return f.name }
func (f *fileInfo) Size() int64        { return f.size }
func (f *fileInfo) Sys() interface{}   { return nil }

// Reader can read ar archives
type Reader struct {
	buffer  *bufio.Reader
	valid   bool
	err     error
	section io.LimitedReader
	hslice  []byte
}

// Reset cancels all internal state/buffering and starts to read from in.
// Useful to avoid allocations, but otherwise has the same effect as r := NewReader(in)
func (r *Reader) Reset(in io.Reader) {
	r.buffer.Reset(in)
	r.valid = false
	r.err = nil
	r.section.R, r.section.N = nil, 0
}

// NewReader will start parsing a possible archive from r
func NewReader(r io.Reader) *Reader {
	reader := &Reader{}
	reader.buffer = bufio.NewReader(r)
	reader.hslice = make([]byte, 60)
	return reader
}

// sticks an error to the reader. From now on this error is returned
// for each following operation until Reset is called.
func (r *Reader) stick(err error) error {
	r.err = err
	return err
}

func (r *Reader) flush_section() error {
	if r.section.R == nil {
		panic("flush_section called, but no section present")
	}

	if r.section.N > 0 {
		_, err := io.Copy(ioutil.Discard, &r.section)
		return r.stick(err)
	}
	// skip padding byte.
	if c, err := r.buffer.ReadByte(); err != nil {
		return r.stick(err)
	} else if c != '\n' {
		// If it wasn't padding, put it back
		r.buffer.UnreadByte()
	}
	r.section.R, r.section.N = nil, 0
	return nil
}

// Next will advance to the next available file in the archive and return it's meta data.
// After calling r.Next, you can use r.Read() to actually read the file contained.
func (r *Reader) Next() (os.FileInfo, error) {
	if r.err != nil {
		return nil, r.err
	}
	if !r.valid {
		if err := checkMagic(r.buffer); err != nil {
			return nil, r.stick(err)
		}

		r.valid = true
	}

	if r.section.R != nil {
		if err := r.flush_section(); err != nil {
			return nil, r.stick(err)
		}
	}

	if _, err := io.ReadFull(r.buffer, r.hslice); err != nil {
		return nil, r.stick(err)
	}

	fi, err := parseFileHeader(r.hslice)
	if err != nil {
		return nil, r.stick(err)
	}
	r.section.R, r.section.N = r.buffer, fi.Size()
	return fi, nil
}

func (r *Reader) Read(b []byte) (n int, err error) {
	if r.err != nil {
		return 0, r.err
	}
	if r.section.R != nil {
		return r.section.Read(b)
	}

	return 0, os.ErrNotExist
}

// NotImplementedError will be returned for any features not implemented in this package.
// It means the archive may be valid, but it uses features detected and not (yet) supported by this archive
type NotImplementedError string

func (feature NotImplementedError) Error() string {
	return "feature not implemented: " + string(feature)
}

// CorruptArchiveError will be returned, if this archive cannot be parsed.
type CorruptArchiveError string

func (c CorruptArchiveError) Error() string {
	return "corrupt archive: " + string(c)
}

func parseFileMode(s string) (filemode os.FileMode, err error) {
	mode, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return filemode, CorruptArchiveError(err.Error())
	}

	if os.FileMode(mode) != (os.FileMode(mode) & (os.ModePerm | syscall.S_IFMT)) {
		return filemode, CorruptArchiveError("invalid file mode")
	}

	switch mode & syscall.S_IFMT {
	case 0: // no file type sepcified, assume regular file
	case syscall.S_IFREG: // regular file, nothing to add
	default:
		return filemode, NotImplementedError("non-regular files")
	}

	return os.FileMode(mode) & os.ModePerm, nil
}

func parseFileHeader(header []byte) (*fileInfo, error) {
	if len(header) != 60 {
		panic("invalid file header")
	}

	if header[58] != filemagic[0] || header[59] != filemagic[1] {
		return nil, CorruptArchiveError("per file magic not found")
	}

	name := string(bytes.TrimSpace(header[0:16]))
	secs, err := strconv.ParseInt(string(bytes.TrimSpace(header[16:16+12])), 10, 64)
	if err != nil {
		return nil, CorruptArchiveError(err.Error())
	}

	filemode, err := parseFileMode(string(bytes.TrimSpace(header[40 : 40+8])))
	if err != nil {
		return nil, err
	}

	filesize, err := strconv.ParseInt(string(bytes.TrimSpace(header[48:48+10])), 10, 64)
	if err != nil {
		return nil, CorruptArchiveError(err.Error())
	}

	fi := &fileInfo{
		name:  name,
		mtime: time.Unix(secs, 0),
		mode:  filemode,
		size:  filesize,
	}

	return fi, nil
}

func checkMagic(r io.Reader) error {
	m := make([]byte, len(magic))
	_, err := io.ReadFull(r, m)
	if err != nil {
		return err
	}

	if string(m) != magic {
		return CorruptArchiveError("global archive header not found")
	}

	return nil
}

// Writer can write ar archives
type Writer struct {
	buffer *bufio.Writer
	err    error
	valid  bool
}

// NewWriter returns an archive writer, which writes to w
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		buffer: bufio.NewWriter(w),
	}
}

// sticks an error to the writer. From now on this error is returned
// for each following operation until Reset is called.
func (w *Writer) stick(err error) error {
	w.err = err
	return err
}

func (w *Writer) writeArchiveHeader() (n int, err error) {
	if w.valid {
		return 0, nil
	}
	n, err = w.buffer.WriteString(magic)
	if err == nil {
		w.valid = true
	}
	return n, err
}
func (w *Writer) writeFileHeader(meta os.FileInfo) (n int, err error) {
	name := meta.Name()
	if len(name) > 16 {
		return 0, NotImplementedError("file names longer than 16 bytes are not supported")
	}

	h := make([]byte, 60)
	for i := range h {
		h[i] = ' '
	}
	copy(h[0:], name)
	copy(h[16:], strconv.FormatInt(meta.ModTime().Unix(), 10))
	copy(h[28:], "0")
	copy(h[34:], "0")
	copy(h[40:], strconv.FormatUint(uint64(meta.Mode()), 8))
	copy(h[48:], strconv.FormatInt(meta.Size(), 10))
	copy(h[58:], filemagic)

	return w.buffer.Write(h)
}

// WriteFile returns how much it has been written or an error, if one occured
func (w *Writer) WriteFile(meta os.FileInfo, r io.Reader) (written int64, err error) {
	if w.err != nil {
		return 0, w.err
	}

	n, err := w.writeArchiveHeader()
	written += int64(n)
	if err != nil {
		return written, w.stick(err)
	}

	n, err = w.writeFileHeader(meta)
	written += int64(n)
	if err != nil {
		return written, w.stick(err)
	}

	copied, err := io.CopyN(w.buffer, r, meta.Size())
	written += int64(copied)

	if err != nil {
		return written, w.stick(err)
	}

	// padding on odd offsets in the archive
	if written%2 == 1 {
		err = w.buffer.WriteByte('\n')
		if err != nil {
			return written, w.stick(err)
		}
		written += 1
	}

	err = w.buffer.Flush()
	return written, w.stick(err)
}
