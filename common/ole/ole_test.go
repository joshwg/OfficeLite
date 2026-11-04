// ole_test.go tests that corrupt compound-file headers are rejected.

package ole

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// corrupt returns a small valid compound file with its header patched.
func corrupt(t *testing.T, patch func(h []byte)) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, []Stream{{Name: "WordDocument", Data: []byte("hello")}}); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	patch(data)
	return data
}

// TestCorruptHeaderRejected checks that headers with impossible sector
// counts, versions or sector shifts are rejected before mscfb sees them
// (one claiming 0xFFFFFFFF directory sectors made it allocate a
// 4-billion-entry slice).
func TestCorruptHeaderRejected(t *testing.T) {
	put32 := func(off int, v uint32) func([]byte) {
		return func(h []byte) { binary.LittleEndian.PutUint32(h[off:], v) }
	}
	cases := map[string]func([]byte){
		"v3 directory sectors": put32(40, 0xFFFFFFFF),
		"v4 directory sectors": func(h []byte) {
			binary.LittleEndian.PutUint16(h[26:], 4)
			binary.LittleEndian.PutUint16(h[30:], 12)
			binary.LittleEndian.PutUint32(h[40:], 0xFFFFFFFF)
		},
		"FAT sectors":      put32(44, 0x7FFFFFFF),
		"mini FAT sectors": put32(64, 0xFFFFFFFF),
		"DIFAT sectors":    put32(72, 0x10000000),
		"version":          func(h []byte) { binary.LittleEndian.PutUint16(h[26:], 5) },
		"sector shift":     func(h []byte) { binary.LittleEndian.PutUint16(h[30:], 12) },
	}
	for name, patch := range cases {
		data := corrupt(t, patch)
		if _, err := Streams(data); err == nil {
			t.Errorf("%s: Streams accepted a corrupt header", name)
		}
		if _, _, err := ReadStream(data, "WordDocument"); err == nil {
			t.Errorf("%s: ReadStream accepted a corrupt header", name)
		}
	}
	if _, err := Streams(corrupt(t, func([]byte) {})); err != nil {
		t.Errorf("valid file rejected: %v", err)
	}
	if _, err := Streams(Magic); err == nil {
		t.Error("truncated header accepted")
	}
}

// TestLoopedStreamChain checks that a stream whose sector chain loops back
// on itself under a size far beyond the file is refused: mscfb follows
// the loop for as long as the size says, so a few patched bytes once made
// ReadStream read (and allocate) gigabytes.
func TestLoopedStreamChain(t *testing.T) {
	var buf bytes.Buffer
	big := bytes.Repeat([]byte("x"), 4096) // 8 sectors, in the FAT (not the mini stream)
	if err := Write(&buf, []Stream{{Name: "WordDocument", Data: big}}); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	le := binary.LittleEndian
	sectorAt := func(n uint32) int { return int(n+1) * 512 }
	fat := sectorAt(le.Uint32(data[0x4C:]))
	le.PutUint32(data[fat+7*4:], 0) // the stream's last sector → its first
	dir := sectorAt(le.Uint32(data[0x30:]))
	le.PutUint32(data[dir+128+0x78:], 64<<20) // entry 1's size: 64 MiB
	if _, b, err := ReadStream(data, "WordDocument"); err == nil {
		t.Errorf("read %d bytes of a looped chain from a %d-byte file", len(b), len(data))
	}
}
