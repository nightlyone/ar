package ar

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"syscall"
	"time"
)

const (
	Magic     = "!<arch>\n"
	FileMagic = "\x60\x0A"
)

type file struct {
	name [16]uint8 // Filename in  ASCII

}

type fileInfo struct {
	name         string
	owner, group string
	mode         os.FileMode
	size         int64
	mtime        time.Time
}

// NotImplemented will be returned for any features not implemented in this package.
// It means the archive may be valid, but it uses features detected and not (yet) supported by this archive
type NotImplemented string

func (feature NotImplemented) Error() string {
	return "feature not implemented: " + string(feature)
}

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

	owner, err := strconv.Atoi(string(bytes.TrimSpace(fh[28 : 28+6])))
	if err != nil {
		return nil, CorruptArchive(err.Error())
	}
	group, err := strconv.Atoi(string(bytes.TrimSpace(fh[34 : 34+6])))
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
		owner: strconv.Itoa(owner),
		group: strconv.Itoa(group),
		mtime: time.Unix(secs, 0),
		mode:  filemode,
		size:  filesize,
	}

	return fi, nil
}

func hasMagic(r io.Reader) bool {
	m := make([]byte, len(Magic))
	_, err := io.ReadFull(r, m)
	if err != nil {
		return false
	}

	return string(m) == Magic
}
