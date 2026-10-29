// Tests for the Windows file dialog's filter string.

package gui

import (
	"testing"
	"unsafe"
)

// TestFilterSpec checks that no filters give a nil spec and that a spec
// ends in a double NUL.
func TestFilterSpec(t *testing.T) {
	if p := filterSpec(nil); p != nil {
		t.Fatal("no filters: want a nil filter")
	}
	p := filterSpec([]FileFilter{{Name: "Text", Exts: []string{".txt"}}})
	// Read up to the double NUL, as the dialog does.
	var got []uint16
	for i := 0; ; i++ {
		c := *(*uint16)(unsafe.Add(unsafe.Pointer(p), 2*i))
		got = append(got, c)
		if c == 0 && i > 0 && got[i-1] == 0 {
			break
		}
	}
	if n := len(got); n < 4 || got[n-1] != 0 || got[n-2] != 0 {
		t.Fatalf("spec %v isn't double-NUL terminated", got)
	}
}
