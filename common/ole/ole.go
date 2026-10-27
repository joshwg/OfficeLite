// Package ole reads streams out of OLE2 compound files, the container used by
// the binary Office formats (.xls, .doc, .ppt).
package ole

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/richardlehane/mscfb"
)

// Magic is the compound-file signature.
var Magic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// IsCompoundFile reports whether data starts with the OLE2 signature.
func IsCompoundFile(data []byte) bool { return bytes.HasPrefix(data, Magic) }

// Streams returns the names of all streams in the file (case preserved).
func Streams(data []byte) ([]string, error) {
	r, err := mscfb.New(bytes.NewReader(data))
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
	r, err := mscfb.New(bytes.NewReader(data))
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
