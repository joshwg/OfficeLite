// Tests for OpenFile's routing: missing files, raised windows, pristine
// windows, new windows and the other app.

package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/doctype"
	"officelite/common/recent"
)

// openLog records what an OpenRequest's callbacks were asked to do.
type openLog struct {
	here     []string
	launched []string
	kinds    []doctype.Kind
}

// request is an OpenRequest for path from a Word-like window whose
// pristine state is given, recording into l.
func (l *openLog) request(path string, pristine bool, rec *recent.List) OpenRequest {
	return OpenRequest{
		Path:     path,
		Recent:   rec,
		Own:      doctype.Document,
		Takes:    func(k doctype.Kind) bool { return k == doctype.Document },
		Pristine: func() bool { return pristine },
		OpenHere: func(p string) { l.here = append(l.here, p) },
		Launch: func(k doctype.Kind, p string) {
			l.kinds = append(l.kinds, k)
			l.launched = append(l.launched, p)
		},
		Activate: func(string) bool { return false },
	}
}

// TestOpenFileRouting checks each route: a pristine window takes its own
// kind, a modified one launches a new window, the other app's file goes
// there, a missing file is reported and dropped from the recent files,
// and a file open elsewhere only raises that window.
func TestOpenFileRouting(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "a.txt")
	csv := filepath.Join(dir, "b.csv")
	for _, p := range []string{txt, csv} {
		if err := os.WriteFile(p, []byte("x,y\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	kTxt, _ := doctype.DetectFile(txt)
	kCSV, _ := doctype.DetectFile(csv)
	if kTxt != doctype.Document || kCSV != doctype.Spreadsheet {
		t.Skipf("detection: %v %v", kTxt, kCSV)
	}

	var l openLog
	if msg := OpenFile(l.request(txt, true, nil)); msg != "" || len(l.here) != 1 || len(l.launched) != 0 {
		t.Errorf("pristine: msg %q here %v launched %v", msg, l.here, l.launched)
	}
	l = openLog{}
	if OpenFile(l.request(txt, false, nil)); len(l.here) != 0 || len(l.launched) != 1 || l.kinds[0] != doctype.Document {
		t.Errorf("modified: here %v launched %v %v", l.here, l.launched, l.kinds)
	}
	l = openLog{}
	if OpenFile(l.request(csv, true, nil)); len(l.here) != 0 || len(l.kinds) != 1 || l.kinds[0] != doctype.Spreadsheet {
		t.Errorf("other app: here %v kinds %v", l.here, l.kinds)
	}

	rec := recent.Open(filepath.Join(dir, "recent.json"))
	missing := filepath.Join(dir, "gone.docx")
	if err := rec.Add(missing, recent.Document); err != nil {
		t.Fatal(err)
	}
	l = openLog{}
	msg := OpenFile(l.request(missing, true, rec))
	if !strings.HasPrefix(msg, "Can't open gone.docx") || len(l.here)+len(l.launched) != 0 {
		t.Errorf("missing: msg %q here %v launched %v", msg, l.here, l.launched)
	}
	if len(rec.Entries()) != 0 {
		t.Errorf("missing file still in the recent list: %v", rec.Entries())
	}

	l = openLog{}
	r := l.request(txt, true, nil)
	r.Activate = func(string) bool { return true }
	if OpenFile(r); len(l.here)+len(l.launched) != 0 {
		t.Errorf("open elsewhere: here %v launched %v", l.here, l.launched)
	}
}
