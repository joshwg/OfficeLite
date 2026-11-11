// Tests for the shared locking glue: title marks and the File In Use
// dialog's answers.

package gui

import (
	"testing"

	"officelite/common/lockfile"
)

// TestDocTitle checks the read-only mark wins over the recovered one.
func TestDocTitle(t *testing.T) {
	for _, c := range []struct {
		ro, rec bool
		want    string
	}{
		{false, false, "A.docx"},
		{true, false, "A.docx (Read-Only)"},
		{false, true, "A.docx [Recovered]"},
		{true, true, "A.docx (Read-Only)"},
	} {
		if got := DocTitle("A.docx", c.ro, c.rec); got != c.want {
			t.Errorf("DocTitle(%v, %v) = %q, want %q", c.ro, c.rec, got, c.want)
		}
	}
}

// TestAskLocked checks the dialog shows the status's title and buttons
// and that each button (and Esc) gives its choice.
func TestAskLocked(t *testing.T) {
	var dlg Dialog
	st := lockfile.Status{Owner: "Alice"}
	var got []lockfile.Choice
	answer := AskLocked(&dlg, st, "/x/Budget.xlsx", func(c lockfile.Choice) { got = append(got, c) })
	if !dlg.Visible || dlg.Title != st.Title() || len(dlg.Buttons) != 3 {
		t.Fatalf("dialog: %+v", dlg)
	}
	for _, i := range []int{0, 1, 2, -1} {
		answer(i)
	}
	want := []lockfile.Choice{lockfile.ReadOnly, lockfile.Anyway, lockfile.Cancel, lockfile.Cancel}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("button %d: %v, want %v", i, got[i], want[i])
		}
	}
}
