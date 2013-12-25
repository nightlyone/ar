package ar

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// test whether fileInfo implements os.FileInfo
var _ os.FileInfo = new(fileInfo)

var testCommon = "!<arch>\n" +
	"debian-binary   1385068169  0     0     644     4         `\n" +
	"2.0\n" +
	"control.tar.gz  1385068169  0     0     644     0         `\n"

var testCommonFileHeaders = []struct {
	in   string
	want *fileInfo
	err  string
}{
	{
		in: "debian-binary   1385068169  0     0     100644  4         `\n",
		want: &fileInfo{
			name:  "debian-binary",
			mtime: time.Unix(1385068169, 0),
			mode:  os.FileMode(0100644) & os.ModePerm,
			size:  4,
		},
	},
	{
		in: "debian-binary   1385068169  0     0     644     4         `\n",
		want: &fileInfo{
			name:  "debian-binary",
			mtime: time.Unix(1385068169, 0),
			mode:  os.FileMode(0644),
			size:  4,
		},
	},
	{
		in:  "debian-binary   1385068169  0     0     120644  4         `\n",
		err: "feature not implemented: non-regular files",
	},
	{
		in:  "debian-binary   1385068169  0     0     220644  4         `\n",
		err: "corrupt archive: invalid file mode",
	},
}

func TestReadFileHeader(t *testing.T) {
	for i, test := range testCommonFileHeaders {
		got, err := parseFileHeader([]byte(test.in))
		switch {
		case err == nil && test.err != "":
			t.Errorf("%d: got no err, expected err %v", i, test.err)
			continue
		case err != nil && test.err != err.Error():
			t.Errorf("%d: got err %q, expected err %q", i, err, test.err)
			continue
		case err == nil && test.err == "":
			// no error as expected
		case err != nil && test.err == err.Error():
			t.Logf("%d: got expected error %q", i, err)
			continue
		}

		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d: got %#v, expected %+v", i, got, test.want)
		} else {
			t.Logf("%d: got %#v", i, got)
		}
	}
}

func BenchmarkReadFileHeader(b *testing.B) {
	fh := []byte("debian-binary   1385068169  0     0     100644  4         `\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseFileHeader(fh)
		b.SetBytes(60)
	}

}

var testMagic = []struct {
	in   string
	want error
}{
	{
		in: magic,
	},
	{
		in:   strings.Repeat("a", len(magic)),
		want: CorruptArchiveError("global archive header not found"),
	},
	{
		in:   "!",
		want: io.ErrUnexpectedEOF,
	},
	{
		in:   "",
		want: io.EOF,
	},
}

func TestReadMagic(t *testing.T) {
	for i, test := range testMagic {
		got := checkMagic(strings.NewReader(test.in))
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d: got %#v, expected %+v", i, got, test.want)
		} else {
			t.Logf("%d: got %#v", i, got)
		}
	}
}

func TestFileInfo(t *testing.T) {
	test := &fileInfo{
		name:  "debian-binary",
		mtime: time.Unix(1385068169, 0),
		mode:  os.FileMode(0644),
		size:  4,
	}

	if test.IsDir() != false {
		t.Error("IsDir")
	}
	if test.Mode() != os.FileMode(0644) {
		t.Error("Mode")
	}
	if test.ModTime() != time.Unix(1385068169, 0) {
		t.Error("ModTime")
	}
	if test.Name() != "debian-binary" {
		t.Error("Name")
	}
	if test.Size() != 4 {
		t.Error("Size")
	}
	if test.Sys() != nil {
		t.Error("Sys")
	}
}

func TestReaderBasics(t *testing.T) {
	test := strings.NewReader(testCommon)
	r := NewReader(test)
	fi, err := r.Next()
	if err != nil {
		t.Fatal(err)
		return
	}
	if fi.Mode() != os.FileMode(0644) {
		t.Error("Mode")
	}
	if fi.Name() != "debian-binary" {
		t.Error("Name")
	}
	if fi.Size() != 4 {
		t.Error("Size")
	}
	if fi.ModTime() != time.Unix(1385068169, 0) {
		t.Error("ModTime")
	}

	if content, err := ioutil.ReadAll(r); err != nil {
		t.Error(err)
	} else if string(content) != "2.0\n" {
		t.Error("Content")
	}
	fi, err = r.Next()
	if err != nil {
		t.Fatal(err)
		return
	}
	if fi.Mode() != os.FileMode(0644) {
		t.Error("Mode2")
	}
	if fi.Name() != "control.tar.gz" {
		t.Error("Name2")
	}
	if fi.Size() != 0 {
		t.Error("Size2")
	}
	if fi.ModTime() != time.Unix(1385068169, 0) {
		t.Error("ModTime2")
	}

	if content, err := ioutil.ReadAll(r); err != nil {
		t.Error(err)
	} else if string(content) != "" {
		t.Error("Content2")
	}

	fi, err = r.Next()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func BenchmarkReaderBigFiles(b *testing.B) {
	benchmarkReader(b, 8, 8*1024*1024)
}

func BenchmarkReaderManySmallFiles(b *testing.B) {
	benchmarkReader(b, 1024, 8)
}

func genArchiveFile(meta os.FileInfo) []byte {
	ret := make([]byte, 60+meta.Size(), 60+meta.Size())
	for i := 0; i < 60; i++ {
		ret[i] = ' '
	}
	copy(ret[0:], meta.Name())
	copy(ret[16:], strconv.FormatInt(meta.ModTime().Unix(), 10))
	copy(ret[28:], "0")
	copy(ret[34:], "0")
	copy(ret[40:], strconv.FormatUint(uint64(meta.Mode()), 8))
	copy(ret[48:], strconv.FormatInt(meta.Size(), 10))
	copy(ret[58:], filemagic)
	return ret
}

func benchmarkReader(b *testing.B, numFiles int, sizeFiles int64) {
	buf := bytes.NewBufferString(magic)
	buf.Grow(numFiles * (int(sizeFiles) + 60))
	for i := 0; i < numFiles; i++ {
		buf.Write(genArchiveFile(&fileInfo{
			name:  strconv.Itoa(1000 + i),
			mtime: time.Unix(int64(i), 0),
			size:  sizeFiles,
			mode:  os.FileMode(0640),
		}))

	}
	test := bytes.NewReader(buf.Bytes())
	r := NewReader(test)

	var err error
	var read int64

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < numFiles; j++ {
			_, err = r.Next()
			if err != nil {
				b.Fatal(err)
			}
			read += 60
			n, err := io.Copy(ioutil.Discard, r)
			if err != nil {
				b.Fatal(err)
			}
			read += n
		}
		b.SetBytes(read)
		read = 0
		test.Seek(0, 0)
		r.Reset(test)
	}
}

func TestWriterBasics(t *testing.T) {
	b := new(bytes.Buffer)
	b.Grow(len(testCommon))
	w := NewWriter(b)
	debian := &fileInfo{
		name:  "debian-binary",
		mtime: time.Unix(1385068169, 0),
		mode:  os.FileMode(0100644) & os.ModePerm,
		size:  4,
	}
	if _, err := w.WriteFile(debian, strings.NewReader("2.0\n")); err != nil {
		t.Error(err)
		return
	}

	control := &fileInfo{
		name:  "control.tar.gz",
		mtime: time.Unix(1385068169, 0),
		mode:  os.FileMode(0100644) & os.ModePerm,
		size:  0,
	}
	if _, err := w.WriteFile(control, strings.NewReader("")); err != nil {
		t.Error(err)
		return
	}

	if archive := b.String(); archive != testCommon {
		t.Errorf("got\n%q\nwant\n%q", archive, testCommon)
	}
}

type fileEntry struct {
	meta  *fileInfo
	input *bytes.Reader
}

type fileSet []*fileEntry

func benchmarkWriter(b *testing.B, numFiles int, sizeFiles int64) {
	zero := make([]byte, len(magic)+numFiles*(len(filemagic)+int(sizeFiles)))
	fs := make(fileSet, 0, numFiles)
	for i := 0; i < numFiles; i++ {
		fs = append(fs, &fileEntry{
			meta: &fileInfo{
				name:  strconv.Itoa(1000 + i),
				mtime: time.Unix(int64(i), 0),
				size:  sizeFiles,
				mode:  os.FileMode(0640),
			},
			input: bytes.NewReader(zero),
		})

	}

	dest := new(bytes.Buffer)
	dest.Grow(len(magic) + numFiles*(len(filemagic)+int(sizeFiles)))
	w := NewWriter(dest)

	var written int64

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, fe := range fs {
			meta, input := fe.meta, fe.input
			input.Seek(0, 0)
			n, err := w.WriteFile(meta, input)
			if err != nil {
				b.Fatal(err)
				return
			}
			written += n
		}
		b.SetBytes(written)
		written = 0
		dest.Reset()
		w.Reset(dest)
	}
}

func BenchmarkWriterBigFiles(b *testing.B) {
	benchmarkWriter(b, 8, 8*1024*1024)
}

func BenchmarkWriterManySmallFiles(b *testing.B) {
	benchmarkWriter(b, 1024, 8)
}
