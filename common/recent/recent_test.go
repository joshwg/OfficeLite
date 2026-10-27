package recent

import (
	"path/filepath"
	"testing"
)

func TestList(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sub", "recent.json")
	l := Open(store)
	l.Add("/a/one.xlsx", Spreadsheet)
	l.Add("/a/two.docx", Document)
	l.Add("/a/one.xlsx", Spreadsheet) // moves to the top, no duplicate
	got := l.Entries()
	if len(got) != 2 || got[0].Name() != "one.xlsx" || got[1].Kind != Document {
		t.Fatalf("entries = %+v", got)
	}

	reloaded := Open(store)
	if e := reloaded.Entries(); len(e) != 2 || e[0].Path != got[0].Path {
		t.Fatalf("reloaded = %+v", e)
	}
	reloaded.Remove("/a/one.xlsx")
	if e := reloaded.Entries(); len(e) != 1 || e[0].Name() != "two.docx" {
		t.Fatalf("after remove = %+v", e)
	}
	for i := 0; i < Max+5; i++ {
		l.Add(filepath.Join("/f", string(rune('a'+i%26)), "x.csv"), Spreadsheet)
	}
	if n := len(l.Entries()); n > Max {
		t.Errorf("len = %d, want <= %d", n, Max)
	}
}
