// Tests for OpenURL's link filter.

package gui

import "testing"

// TestOpenURLRefusesPrograms checks that OpenURL refuses programs, scripts
// and unknown schemes, and that linkAllowed passes web, e-mail and
// document links.
func TestOpenURLRefusesPrograms(t *testing.T) {
	for _, u := range []string{
		`C:\Windows\System32\calc.exe`, `c:\tools\RUN.BAT`, `\\server\share\setup.MSI`,
		`calc.exe. `, `report.pdf:evil.exe`, "file:///C:/Windows/notepad.exe",
		"file:///C:/a%20b/x.vbs", "javascript:alert(1)", "ms-settings:display", "",
		// Windows resolves these to the program.
		`C:\Windows\System32\calc.exe\.`, `C:\Windows\System32\calc.exe/.`, "file:///C:/Windows/System32/calc.exe/.",
		`C:\Windows\System32\calc.exe\`, `C:\tools\x\..\RUN.BAT`, `C:\Windows\calc.exe\.\.`, "C:calc.exe",
	} {
		if err := OpenURL(u); err != errUnsafeLink {
			t.Errorf("OpenURL(%q) = %v, want refused", u, err)
		}
	}
	for _, u := range []string{
		"https://example.com", "HTTP://x", " mailto:a@b.c", "ftp://h/f",
		`C:\Users\josh\Documents\Budget.xlsx`, `\\server\share\Report.docx`, "Notes.pdf",
		"file:///C:/Users/josh/My%20Docs/plan.docx", `C:\Users\josh\Documents`,
		// Folders, drive roots included.
		`C:\`, "C:", "d:/", "file:///C:/", `C:\Users\josh\Documents\.`, `\\server\share\`,
	} {
		if !linkAllowed(u) {
			t.Errorf("linkAllowed(%q) = false, want allowed", u)
		}
	}
}
