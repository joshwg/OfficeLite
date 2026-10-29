// Package ole reads streams out of OLE2 compound files, the container used by
// the binary Office formats (.xls, .doc, .ppt).
package ole

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/richardlehane/mscfb"
)

// Magic is the compound-file signature.
var Magic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// IsCompoundFile reports whether data starts with the OLE2 signature.
func IsCompoundFile(data []byte) bool { return bytes.HasPrefix(data, Magic) }

// Open validates the compound-file header and opens the directory. The
// header's sector counts are checked against the file size first: mscfb
// trusts them and sizes allocations from them, so a few corrupt header
// bytes could otherwise ask for gigabytes.
func Open(r io.ReaderAt, size int64) (*mscfb.Reader, error) {
	if err := CheckHeader(r, size); err != nil {
		return nil, err
	}
	return mscfb.New(r)
}

// CheckHeader sanity-checks the 512-byte compound-file header of a file of
// size bytes: version, sector size and that every sector count fits in the
// file.
func CheckHeader(r io.ReaderAt, size int64) error {
	h := make([]byte, 512)
	if size < int64(len(h)) {
		return errors.New("compound file too short")
	}
	if _, err := r.ReadAt(h, 0); err != nil {
		return err
	}
	if !IsCompoundFile(h) {
		return errors.New("bad compound file signature")
	}
	u16 := func(off int) uint16 { return binary.LittleEndian.Uint16(h[off:]) }
	u32 := func(off int) uint64 { return uint64(binary.LittleEndian.Uint32(h[off:])) }
	major, shift := u16(26), u16(30)
	switch {
	case major == 3 && shift == 9, major == 4 && shift == 12:
	default:
		return fmt.Errorf("unsupported compound file version %d (sector shift %d)", major, shift)
	}
	sector := uint64(1) << shift
	if major == 3 && u32(40) != 0 {
		return errors.New("bad compound file header (directory sector count)")
	}
	// Directory (offset 40), FAT (44), mini FAT (64) and DIFAT (72)
	// sectors all live in the file, so none of the counts can exceed it.
	for _, off := range []int{40, 44, 64, 72} {
		if u32(off)*sector > uint64(size) {
			return fmt.Errorf("bad compound file header (sector count at %d exceeds the file)", off)
		}
	}
	return nil
}

// Streams returns the names of all streams in the file (case preserved).
func Streams(data []byte) ([]string, error) {
	r, err := Open(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var names []string
	for e, err := r.Next(); err == nil; e, err = r.Next() {
		names = append(names, e.Name)
	}
	return names, nil
}

// ReadStream returns the contents of the first stream whose name matches one
// of names (case-insensitive), in order of preference.
func ReadStream(data []byte, names ...string) (name string, content []byte, err error) {
	r, err := Open(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", nil, fmt.Errorf("not a valid compound file: %w", err)
	}
	found := map[string][]byte{}
	for e, err := r.Next(); err == nil; e, err = r.Next() {
		for _, n := range names {
			if strings.EqualFold(e.Name, n) && found[n] == nil {
				b, err := io.ReadAll(e)
				if err != nil {
					return "", nil, err
				}
				found[n] = b
			}
		}
	}
	for _, n := range names {
		if b, ok := found[n]; ok {
			return n, b, nil
		}
	}
	return "", nil, fmt.Errorf("stream %q not found", names)
}
