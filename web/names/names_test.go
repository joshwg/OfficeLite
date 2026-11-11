// Tests of the username and file-name rules.

package names

import (
	"strings"
	"testing"

	"officelite/web"
)

// TestUsername checks which usernames pass.
func TestUsername(t *testing.T) {
	long := strings.Repeat("a", web.UsernameMaxLength)
	for _, name := range []string{"ann", "Josh", "j.smith", "a_b-c", "007", long} {
		if err := Username(name); err != nil {
			t.Errorf("Username(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", ".hidden", "-x", "_x", "a b", "x/y", "../etc",
		"a@b", "josé", "a\x00b", "tab\tname", long + "a"} {
		if Username(name) == nil {
			t.Errorf("Username(%q) accepted", name)
		}
	}
}

// TestUserDir checks that the folder name ignores case.
func TestUserDir(t *testing.T) {
	if got := UserDir("Josh.Smith"); got != "josh.smith" {
		t.Errorf("UserDir = %q", got)
	}
}

// TestFileName checks trimming and which file names pass.
func TestFileName(t *testing.T) {
	ok := map[string]string{
		"Budget.xlsx":           "Budget.xlsx",
		"  Budget 2026.XLSX  ":  "Budget 2026.XLSX",
		"a+b=c (1) [x] #2.docx": "a+b=c (1) [x] #2.docx",
		"report.pdf":            "report.pdf",
		strings.Repeat("a", web.FileNameMaxLength-5) + ".docx": strings.Repeat("a", web.FileNameMaxLength-5) + ".docx",
	}
	for in, want := range ok {
		got, err := FileName(in)
		if err != nil || got != want {
			t.Errorf("FileName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "   ", ".docx", ".hidden.docx", "~$Budget.xlsx", "a/b.docx", `a\b.docx`, "a*.docx",
		"a\x00.docx", "a\tb.docx", "a\nb.docx", "a\x7fb.docx", "café.docx", "zero​width.docx",
		"photo.png", "noext", "..", strings.Repeat("a", web.FileNameMaxLength-4) + ".docx",
	}
	for _, in := range bad {
		if got, err := FileName(in); err == nil {
			t.Errorf("FileName(%q) accepted as %q", in, got)
		}
	}
}
