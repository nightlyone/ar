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
	Magic     = "!<arch>\n" // Magic value with which ar archives always start
	FileMagic = "\x60\x0A"  // FileMagic is a marker for each per file header to mark it valid
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

type Reader struct {
	buffer  *bufio.Reader
	valid   bool
	err     error
	section io.LimitedReader
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
	return reader
}

// sticks an error to the reader. From now on this error is returned
// for each following operation until Reset is called.
func (r *Reader) stick(err error) error {
	r.err = err
	return err
}

// Next will advance to the next available file in the archive and return it's meta data.
// After calling r.Next, you can use r.Read() to actually read the file contained.
func (r *Reader) Next() (os.FileInfo, error) {
	if r.err != nil {
		return nil, r.err
	}
	if !r.valid {
		err := checkMagic(r.buffer)
		if err != nil {
			return nil, r.stick(err)
		}

		r.valid = true
	}

	if r.section.R != nil {
		if r.section.N > 0 {
			_, err := io.Copy(ioutil.Discard, &r.section)
			return nil, r.stick(err)
		}
		// skip padding byte.
		if c, err := r.buffer.ReadByte(); err != nil {
			return nil, r.stick(err)
		} else if c != '\n' {
			// If it wasn't padding, put it back
			r.buffer.UnreadByte()
		}
		r.section.R, r.section.N = nil, 0
	}

	fi, err := readFileHeader(r.buffer)
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

// NotImplemented will be returned for any features not implemented in this package.
// It means the archive may be valid, but it uses features detected and not (yet) supported by this archive
type NotImplemented string

func (feature NotImplemented) Error() string {
	return "feature not implemented: " + string(feature)
}

// CorruptArchive will be returned, if this archive cannot be parsed.
type CorruptArchive string

func (c CorruptArchive) Error() string {
	return "corrupt archive: " + string(c)
}

func parseFileMode(s string) (filemode os.FileMode, err error) {
	mode, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return filemode, CorruptArchive(err.Error())
	}

	if os.FileMode(mode) != (os.FileMode(mode) & (os.ModePerm | syscall.S_IFMT)) {
		return filemode, CorruptArchive("invalid file mode")
	}

	switch mode & syscall.S_IFMT {
	case 0: // no file type sepcified, assume regular file
	case syscall.S_IFREG: // regular file, nothing to add
	default:
		return filemode, NotImplemented("non-regular files")
	}

	return os.FileMode(mode) & os.ModePerm, nil
}

func readFileHeader(r io.Reader) (*fileInfo, error) {
	fh := make([]byte, 60)
	_, err := io.ReadFull(r, fh)
	if err != nil {
		return nil, err
	}

	if string(fh[58:58+2]) != FileMagic {
		return nil, CorruptArchive("file magic \"" + FileMagic + "\" not found")
	}

	name := string(bytes.TrimSpace(fh[0:16]))
	secs, err := strconv.ParseInt(string(bytes.TrimSpace(fh[16:16+12])), 10, 64)
	if err != nil {
		return nil, CorruptArchive(err.Error())
	}

	filemode, err := parseFileMode(string(bytes.TrimSpace(fh[40 : 40+8])))
	if err != nil {
		return nil, err
	}

	filesize, err := strconv.ParseInt(string(bytes.TrimSpace(fh[48:48+10])), 10, 64)
	if err != nil {
		return nil, CorruptArchive(err.Error())
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
	m := make([]byte, len(Magic))
	_, err := io.ReadFull(r, m)
	if err != nil {
		return err
	}

	if string(m) != Magic {
		return CorruptArchive("global archive header not found")
	}

	return nil
}
