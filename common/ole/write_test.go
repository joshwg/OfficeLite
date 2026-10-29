// write_test.go tests that written compound files read back intact.

package ole

import (
	"bytes"
	"strings"
	"testing"
)

// TestWriteRoundTrip checks that mini, regular, empty and DIFAT-sized
// streams read back as written, and that duplicate names are refused.
func TestWriteRoundTrip(t *testing.T) {
	big := bytes.Repeat([]byte("0123456789abcdef"), 600)          // 9600 bytes: regular sectors
	huge := bytes.Repeat([]byte{0xA5, 0x5A, 0x00, 0x11}, 2200000) // 8.8 MB: needs DIFAT
	streams := []Stream{
		{Name: "Workbook", Data: big},
		{Name: "\x05SummaryInformation", Data: []byte("tiny")},
		{Name: "Small", Data: bytes.Repeat([]byte{7}, 4095)},
		{Name: "Empty", Data: nil},
		{Name: "Huge", Data: huge},
	}
	var buf bytes.Buffer
	if err := Write(&buf, streams); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if !IsCompoundFile(data) {
		t.Fatal("missing signature")
	}
	names, err := Streams(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != len(streams) {
		t.Errorf("streams = %q", names)
	}
	for _, s := range streams {
		if len(s.Data) == 0 {
			continue
		}
		// mscfb drops the control-character prefix of property streams.
		_, got, err := ReadStream(data, strings.TrimLeft(s.Name, "\x05"))
		if err != nil {
			t.Fatalf("%q: %v", s.Name, err)
		}
		if !bytes.Equal(got, s.Data) {
			t.Errorf("%q: got %d bytes, want %d", s.Name, len(got), len(s.Data))
		}
	}
	if err := Write(&buf, []Stream{{Name: "a"}, {Name: "A"}}); err == nil {
		t.Error("duplicate names accepted")
	}
}
