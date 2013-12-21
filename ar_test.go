package ar

import (
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// test whether fileInfo implements os.FileInfo
var _ os.FileInfo = new(fileInfo)

var testCommon = "!<arch>\n" +
	"debian-binary   1385068169  0     0     100644  4         `\n" +
	"2.0\n" +
	"control.tar.gz  1385068169  0     0     100644  0         `\n"

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
		got, err := readFileHeader(strings.NewReader(test.in))
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
	r := strings.NewReader("debian-binary   1385068169  0     0     100644  4         `\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = readFileHeader(r)
		b.SetBytes(60)
		r.Seek(0, 0)
	}

}

var testMagic = []struct {
	in   io.Reader
	want error
}{
	{
		in: strings.NewReader(magic),
	},
	{
		in:   strings.NewReader(strings.Repeat("a", len(magic))),
		want: CorruptArchiveError("global archive header not found"),
	},
	{
		in:   strings.NewReader("!"),
		want: io.ErrUnexpectedEOF,
	},
	{
		in:   strings.NewReader(""),
		want: io.EOF,
	},
}

func TestReadMagic(t *testing.T) {
	for i, test := range testMagic {
		got := checkMagic(test.in)
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

func BenchmarkReader(b *testing.B) {
	// contains 2 files
	test := strings.NewReader(testCommon)
	r := NewReader(test)

	var err error
	var read int64

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 2; j++ {
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
