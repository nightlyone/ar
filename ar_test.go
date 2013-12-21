package ar

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

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
			owner: "0",
			group: "0",
			mtime: time.Unix(1385068169, 0),
			mode:  os.FileMode(0100644) & os.ModePerm,
			size:  4,
		},
	},
	{
		in: "debian-binary   1385068169  0     0     644     4         `\n",
		want: &fileInfo{
			name:  "debian-binary",
			owner: "0",
			group: "0",
			mtime: time.Unix(1385068169, 0),
			mode:  os.FileMode(0644),
			size:  4,
		},
	},
	{
		in:  "debian-binary   1385068169  a     0     100644  4         `\n",
		err: "corrupt archive: strconv.ParseInt: parsing \"a\": invalid syntax",
	},
	{
		in:  "debian-binary   1385068169  0     b     100644  4         `\n",
		err: "corrupt archive: strconv.ParseInt: parsing \"b\": invalid syntax",
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
	for i := 0; i < b.N; i++ {
		_, _ = readFileHeader(r)
		r.Seek(0, 0)
	}

}

var testMagic = []struct {
	in   io.Reader
	want bool
}{
	{
		in:   strings.NewReader(Magic),
		want: true,
	},
	{
		in:   strings.NewReader(strings.Repeat("a", len(Magic))),
		want: false,
	},
}

func TestReadMagic(t *testing.T) {
	for i, test := range testMagic {
		got := hasMagic(test.in)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d: got %#v, expected %+v", i, got, test.want)
		} else {
			t.Logf("%d: got %#v", i, got)
		}
	}
}
