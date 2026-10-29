// Tests for FileFilter's labels and patterns.

package gui

import "testing"

// TestFileFilter checks a filter's label and the all-files pattern.
func TestFileFilter(t *testing.T) {
	if got := WordFilter.Label(); got != "Word Documents (*.docx;*.docm;*.dotx;*.doc;*.txt;*.pages)" {
		t.Errorf("label = %q", got)
	}
	if got := AllFilter.Pattern(); got != "*.*" {
		t.Errorf("all = %q", got)
	}
}
