// Tests for AutoSave: the switch's availability, timed background saves,
// failures and the on-by-default setting.

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"

	"officelite/common/autorecover"
	"officelite/common/autosave"
	"officelite/common/gui"
	"officelite/excel/fileio"
)

// autoSaveX, autoSaveY are where the harness's title bar has the AutoSave switch.
const autoSaveX, autoSaveY = 150, 18

// startAutoSave gives the harness's window an AutoSave saver on a clock
// the test moves, for the workbook saved at path (as .xlsx).
func (h *harness) startAutoSave(path string) *autosave.ManualClock {
	a := h.a
	if path != "" {
		if err := fileio.SaveFile(a.wb, path, fileio.XLSX, 0); err != nil {
			h.t.Fatal(err)
		}
		a.path = path
	}
	a.startAutoSave()
	clock := autosave.NewManualClock()
	a.as.saver.Clock = clock
	h.t.Cleanup(a.as.saver.Close)
	h.frame()
	return clock
}

// settle runs frames until the AutoSave running has written the file and
// its end has been seen.
func (h *harness) settle() {
	for range 3 {
		h.a.as.saver.Wait()
		h.frame()
	}
}

// fileA1 is cell A1 of the workbook in path.
func fileA1(t *testing.T, path string) string {
	t.Helper()
	wb, err := fileio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return wb.Sheets[0].Get(0, 0).Text
}

// TestAutoSaveUnavailable: the switch is grayed, with the reason as its
// tooltip, for a new workbook, a .xls, a read-only or recovered workbook,
// and clicking it does nothing.
func TestAutoSaveUnavailable(t *testing.T) {
	h := newHarness(t)
	a := h.a
	h.startAutoSave("")
	worksWith := "AutoSave works with .xlsx files you've saved. Save a copy as .xlsx to turn it on."
	check := func(what, want string) {
		t.Helper()
		if got := a.autoSaveBlocked(); got != want {
			t.Errorf("%s: %q, want %q", what, got, want)
		}
		h.click(autoSaveX, autoSaveY)
		a.setAutoSave(true)
		if a.as.saver.On() {
			t.Errorf("%s: turned on", what)
		}
	}
	check("new workbook", worksWith)
	dir := t.TempDir()
	for _, name := range []string{"Old.xls", "Data.csv", "Sales.numbers", "Macros.xlsm"} {
		a.path = filepath.Join(dir, name)
		check(name, worksWith)
	}
	a.path = filepath.Join(dir, "Budget.xlsx")
	a.lk.readOnly = true
	check("read-only", "This workbook is read-only. Save a copy as .xlsx to turn AutoSave on.")
	a.lk.readOnly = false
	a.as.elsewhere = true
	check("open anyway", "Someone else has this file open, so AutoSave is off: it would overwrite their changes.")
	a.as.elsewhere = false
	a.ar.recovered = &autorecover.Entry{Name: "Budget.xlsx"}
	check("recovered", "This is a recovered workbook. Save it as .xlsx to turn AutoSave on.")
	a.ar.recovered = nil
	if got := a.autoSaveBlocked(); got != "" {
		t.Errorf("saved .xlsx: %q", got)
	}
	if gui.AutoSaveWorksWith(".xlsx") != worksWith {
		t.Error("tooltip text")
	}
}

// TestAutoSave: turning the switch on saves at once; an edit is saved
// Delay after it (not before), in the background; closing then needs no
// prompt; Ctrl+S still works.
func TestAutoSave(t *testing.T) {
	h := newHarness(t)
	a := h.a
	path := filepath.Join(t.TempDir(), "Budget.xlsx")
	clock := h.startAutoSave(path)
	recDir := t.TempDir()
	h.startRecover(recDir)

	h.typeText("42")
	h.press(key.NameReturn, 0)
	if !a.modified {
		t.Fatal("not modified")
	}
	clock.Advance(time.Minute)
	h.settle()
	if got := fileA1(t, path); got != "" {
		t.Fatalf("saved while off: A1 = %q", got)
	}
	h.click(autoSaveX, autoSaveY) // on: saves now
	if !a.as.saver.On() {
		t.Fatal("the switch didn't turn AutoSave on")
	}
	h.settle()
	if got := fileA1(t, path); got != "42" || a.modified || a.as.savedAt.IsZero() {
		t.Fatalf("turning on: A1 = %q, modified %v", got, a.modified)
	}
	if a.autoSaveStatus(a.gtx) != "Saved" {
		t.Errorf("title status %q", a.autoSaveStatus(a.gtx))
	}

	// An edit: nothing for 2 s, saved at 3 s; the recovery copy goes.
	h.press(key.NameUpArrow, 0)
	h.typeText("43")
	h.press(key.NameReturn, 0)
	h.waitRecover(recDir, 2)
	clock.Advance(2 * time.Second)
	h.settle()
	if got := fileA1(t, path); got != "42" || !a.modified {
		t.Fatalf("before the delay: A1 = %q, modified %v", got, a.modified)
	}
	clock.Advance(time.Second)
	h.settle()
	if got := fileA1(t, path); got != "43" || a.modified {
		t.Fatalf("after the delay: A1 = %q, modified %v", got, a.modified)
	}
	h.waitRecover(recDir, 0)

	// Undo still works across the save (and is saved in turn).
	h.press("Z", key.ModShortcut)
	if h.cell("A1").Text != "42" {
		t.Fatalf("undo: A1 = %q", h.cell("A1").Text)
	}
	clock.Advance(autosave.Delay)
	h.settle()
	if got := fileA1(t, path); got != "42" {
		t.Errorf("undo saved: A1 = %q", got)
	}

	// Nothing unsaved: closing doesn't ask.
	if a.askBeforeClose() {
		t.Error("closing would ask after the last AutoSave")
	}
	// Ctrl+S still saves, and the timer's save after it has nothing to do.
	h.press(key.NameUpArrow, 0)
	h.typeText("44")
	h.press(key.NameReturn, 0)
	h.press("S", key.ModShortcut)
	if got := fileA1(t, path); got != "44" || a.modified {
		t.Errorf("Ctrl+S: A1 = %q, modified %v", got, a.modified)
	}
	clock.Advance(autosave.Delay)
	h.settle()
	if a.as.saver.Busy() || !a.as.saver.On() || a.errMsg != "" {
		t.Errorf("after Ctrl+S: busy %v, on %v, %q", a.as.saver.Busy(), a.as.saver.On(), a.errMsg)
	}

	// Off: edits stay in the window.
	h.click(autoSaveX, autoSaveY)
	if a.as.saver.On() {
		t.Fatal("not turned off")
	}
	h.press(key.NameUpArrow, 0)
	h.typeText("45")
	h.press(key.NameReturn, 0)
	clock.Advance(time.Minute)
	h.settle()
	if got := fileA1(t, path); got != "44" || !a.modified {
		t.Errorf("off: A1 = %q", got)
	}
}

// TestAutoSaveError: a save that fails turns AutoSave off with the error
// in the status bar; the edits stay.
func TestAutoSaveError(t *testing.T) {
	h := newHarness(t)
	a := h.a
	dir := t.TempDir()
	path := filepath.Join(dir, "Budget.xlsx")
	clock := h.startAutoSave(path)
	a.setAutoSave(true)
	h.typeText("42")
	h.press(key.NameReturn, 0)
	os.RemoveAll(dir) // the folder is gone (a USB stick pulled)
	clock.Advance(autosave.Delay)
	h.settle()
	if a.as.saver.On() || !strings.HasPrefix(a.errMsg, "AutoSave turned off: Save failed") || !a.modified || h.cell("A1").Text != "42" {
		t.Errorf("on %v, error %q, modified %v", a.as.saver.On(), a.errMsg, a.modified)
	}
}

// TestAutoSaveDefault: with the default on, a .xlsx opened or saved as
// starts with AutoSave on, other files with it off.
func TestAutoSaveDefault(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	old := autorecover.SettingsPath
	autorecover.SettingsPath = func() string { return settings }
	t.Cleanup(func() { autosave.SetDefault(false); autorecover.SettingsPath = old })
	h := newHarness(t)
	a := h.a
	h.startAutoSave("")
	if autosave.Default() {
		t.Fatal("on by default")
	}
	autosave.SetDefault(true)
	dir := t.TempDir()
	a.saved(a.wb, fileio.XLS, filepath.Join(dir, "Book.xls"), false, nil)
	if a.as.saver.On() {
		t.Error("on for a .xls")
	}
	a.saved(a.wb, fileio.XLSX, filepath.Join(dir, "Book.xlsx"), false, nil)
	if !a.as.saver.On() {
		t.Error("off for a .xlsx saved as")
	}
	a.saved(a.wb, fileio.XLS, filepath.Join(dir, "Book.xls"), false, nil)
	h.frame()
	if a.as.saver.On() {
		t.Error("still on after saving as .xls")
	}
}
