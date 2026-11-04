// fuzz_test.go fuzzes the .doc reader by patching bytes of the
// WordDocument, table and Data streams of the testdata documents; a
// reader panic (reported by Read as an "internal error") fails the test.

package docfile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/ole"
)

// docStreams loads the streams of a testdata document.
func docStreams(tb testing.TB, name string) []ole.Stream {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		tb.Fatal(err)
	}
	names, err := ole.Streams(data)
	if err != nil {
		tb.Fatal(err)
	}
	var out []ole.Stream
	for _, n := range names {
		if n == "Root Entry" {
			continue
		}
		_, b, err := ole.ReadStream(data, n)
		if err != nil {
			continue
		}
		out = append(out, ole.Stream{Name: n, Data: b})
	}
	return out
}

// FuzzRead patches stream bytes of a testdata document (stream picks the
// stream, off the position) and checks Read never panics.
func FuzzRead(f *testing.F) {
	files := []string{"simple.doc", "rich.doc", "complex.doc", "landscape.doc"}
	base := make([][]ole.Stream, len(files))
	for i, n := range files {
		base[i] = docStreams(f, n)
	}
	f.Add(uint8(0), uint8(0), uint32(0), []byte{0})
	f.Add(uint8(1), uint8(1), uint32(0x1A0), []byte{0xFF, 0xFF, 0xFF, 0x7F})
	f.Fuzz(func(t *testing.T, file, stream uint8, off uint32, patch []byte) {
		src := base[int(file)%len(base)]
		streams := make([]ole.Stream, len(src))
		copy(streams, src)
		i := int(stream) % len(streams)
		d := append([]byte(nil), streams[i].Data...)
		if len(d) > 0 {
			o := int(off % uint32(len(d)))
			copy(d[o:], patch)
		}
		streams[i].Data = d
		var buf bytes.Buffer
		if err := ole.Write(&buf, streams); err != nil {
			t.Skip()
		}
		if _, err := Read(buf.Bytes()); err != nil && strings.Contains(err.Error(), "internal error") {
			t.Fatalf("stream %s off %d: %v", streams[i].Name, off, err)
		}
	})
}
