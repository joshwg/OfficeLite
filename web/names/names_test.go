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

// TestFolderName checks the folder rules: the file-name rules, but no
// name ending in a document's extension and none starting with '.'.
func TestFolderName(t *testing.T) {
	for in, want := range map[string]string{
		"Reports": "Reports", "  2026 Q1 ": "2026 Q1", "v1.2": "v1.2", "notes.bak": "notes.bak",
		strings.Repeat("a", web.FileNameMaxLength): strings.Repeat("a", web.FileNameMaxLength),
	} {
		got, err := FolderName(in)
		if err != nil || got != want {
			t.Errorf("FolderName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "  ", ".", "..", ".hidden", ".git", "~$x", "a/b", `a\b`, "a*", "café", "tab\tx",
		"Reports.docx", "Old.XLSX", "x.pdf", "x.txt", strings.Repeat("a", web.FileNameMaxLength+1),
	} {
		if got, err := FolderName(in); err == nil {
			t.Errorf("FolderName(%q) accepted as %q", in, got)
		}
	}
}

// TestFolderPath checks splitting, trimming and the depth limit.
func TestFolderPath(t *testing.T) {
	if got, err := FolderPath(""); err != nil || got != nil {
		t.Errorf(`FolderPath("") = %q, %v`, got, err)
	}
	deepest := strings.TrimSuffix(strings.Repeat("a/", web.FolderMaxDepth), "/")
	for in, want := range map[string]string{"Reports": "Reports", " A / B ": "A|B", deepest: strings.ReplaceAll(deepest, "/", "|")} {
		got, err := FolderPath(in)
		if err != nil || strings.Join(got, "|") != want {
			t.Errorf("FolderPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"/", "A/", "/A", "A//B", "A/../B", "A/.x", "A/b.docx", deepest + "/a"} {
		if got, err := FolderPath(in); err == nil {
			t.Errorf("FolderPath(%q) accepted as %q", in, got)
		}
	}
}

// TestFilePath checks a document's path: its folders and its name.
func TestFilePath(t *testing.T) {
	dirs, name, err := FilePath(" Reports / Budget.xlsx ")
	if err != nil || strings.Join(dirs, "|") != "Reports" || name != "Budget.xlsx" {
		t.Errorf("FilePath = %q, %q, %v", dirs, name, err)
	}
	if dirs, name, err := FilePath("Budget.xlsx"); err != nil || dirs != nil || name != "Budget.xlsx" {
		t.Errorf("FilePath at the top = %q, %q, %v", dirs, name, err)
	}
	deepest := strings.Repeat("a/", web.FolderMaxDepth)
	if _, _, err := FilePath(deepest + "x.docx"); err != nil {
		t.Errorf("FilePath at the deepest: %v", err)
	}
	for _, in := range []string{"/x.docx", " /x.docx", "A/", "A/Reports", "../x.docx", ".A/x.docx", "a.docx/x.docx", deepest + "a/x.docx"} {
		if _, _, err := FilePath(in); err == nil {
			t.Errorf("FilePath(%q) accepted", in)
		}
	}
}
