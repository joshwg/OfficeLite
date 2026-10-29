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

// A header claiming 0xFFFFFFFF directory sectors made mscfb allocate a
// 4-billion-entry slice; it must be rejected instead.
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
