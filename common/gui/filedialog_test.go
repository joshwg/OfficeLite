package gui

import "testing"

func TestFileFilter(t *testing.T) {
	if got := WordFilter.Label(); got != "Word Documents (*.docx;*.docm;*.dotx;*.doc;*.txt)" {
		t.Errorf("label = %q", got)
	}
	if got := AllFilter.Pattern(); got != "*.*" {
		t.Errorf("all = %q", got)
	}
}
