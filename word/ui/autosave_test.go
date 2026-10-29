package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"officelite/common/autorecover"
	"officelite/common/autosave"
	"officelite/word/fileio"
	"officelite/word/model"
)

// autoSaveAt is where the harness's title bar has the AutoSave switch.
var autoSaveAt = f32.Pt(150, 18)

// startAutoSave gives the harness's window an AutoSave saver on a clock
// the test moves, for the document saved at path (as .docx).
func (h *harness) startAutoSave(path string) *autosave.ManualClock {
	d := h.d
	if path != "" {
		if err := fileio.SaveFile(d.doc, path, fileio.DOCX); err != nil {
			h.t.Fatal(err)
		}
		d.path = path
	}
	d.startAutoSave()
	clock := autosave.NewManualClock()
	d.as.saver.Clock = clock
	h.t.Cleanup(d.as.saver.Close)
	h.frame()
	return clock
}

// settle runs the posted calls and frames until the AutoSave running has
// written the file and its end has been seen.
func (h *harness) settle() {
	for range 3 {
		h.d.as.saver.Wait()
		h.d.calls.Run()
		h.frame()
	}
}

// clickAutoSave clicks the title bar's AutoSave switch.
func (h *harness) clickAutoSave() {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: autoSaveAt})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: autoSaveAt})
	h.frame()
}

// fileText is the first paragraph of the document in path.
func fileText(t *testing.T, path string) string {
	t.Helper()
	doc, err := fileio.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return model.NewEditor(doc).Para(0).Text()
}

// TestAutoSaveUnavailable: the switch is greyed, with the reason as its
// tooltip, for a new document, a .doc, a read-only or recovered document,
// and clicking it does nothing.
func TestAutoSaveUnavailable(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.startAutoSave("")
	worksWith := "AutoSave works with .docx files you've saved. Save a copy as .docx to turn it on."
	check := func(what, want string) {
		t.Helper()
		if got := d.autoSaveBlocked(); got != want {
			t.Errorf("%s: %q, want %q", what, got, want)
		}
		h.clickAutoSave()
		d.setAutoSave(true)
		if d.as.saver.On() {
			t.Errorf("%s: turned on", what)
		}
	}
	check("new document", worksWith)
	dir := t.TempDir()
	for _, name := range []string{"Old.doc", "Notes.txt", "Flyer.pages", "Macros.docm"} {
		d.path = filepath.Join(dir, name)
		check(name, worksWith)
	}
	d.path = filepath.Join(dir, "Letter.docx")
	d.lk.readOnly = true
	check("read-only", "This document is read-only. Save a copy as .docx to turn AutoSave on.")
	d.lk.readOnly = false
	d.as.elsewhere = true
	check("open anyway", "Someone else has this file open, so AutoSave is off: it would overwrite their changes.")
	d.as.elsewhere = false
	d.ar.recovered = &autorecover.Entry{Name: "Letter.docx"}
	check("recovered", "This is a recovered document. Save it as .docx to turn AutoSave on.")
	d.ar.recovered = nil
	if got := d.autoSaveBlocked(); got != "" {
		t.Errorf("saved .docx: %q", got)
	}
}

// TestAutoSave: turning the switch on saves at once; typing is saved
// Delay after it stops (not before); undo still works; closing then
// needs no prompt; Ctrl+S still works.
func TestAutoSave(t *testing.T) {
	h := newHarness(t)
	d := h.d
	path := filepath.Join(t.TempDir(), "Letter.docx")
	clock := h.startAutoSave(path)
	recDir := t.TempDir()
	h.startRecover(recDir)

	h.typeText("Dear")
	clock.Advance(time.Minute)
	h.settle()
	if got := fileText(t, path); got != "" {
		t.Fatalf("saved while off: %q", got)
	}
	h.clickAutoSave() // on: saves now
	if !d.as.saver.On() {
		t.Fatal("the switch didn't turn AutoSave on")
	}
	h.settle()
	if got := fileText(t, path); got != "Dear" || d.modified() || d.autoSaveStatus(d.gtx) != "Saved" {
		t.Fatalf("turning on: %q, modified %v", got, d.modified())
	}

	h.typeText(" Sir")
	h.waitRecover(recDir, 2)
	clock.Advance(2 * time.Second)
	h.settle()
	if got := fileText(t, path); got != "Dear" || !d.modified() {
		t.Fatalf("before the delay: %q, modified %v", got, d.modified())
	}
	clock.Advance(time.Second)
	h.settle()
	if got := fileText(t, path); got != "Dear Sir" || d.modified() {
		t.Fatalf("after the delay: %q, modified %v", got, d.modified())
	}
	h.waitRecover(recDir, 0) // the recovery copy goes with the save

	h.press("Z", key.ModShortcut)
	if h.text() != "Dear" {
		t.Fatalf("undo: %q", h.text())
	}
	clock.Advance(autosave.Delay)
	h.settle()
	if got := fileText(t, path); got != "Dear" || d.modified() {
		t.Errorf("undo saved: %q, modified %v", got, d.modified())
	}
	// Nothing unsaved: closing doesn't ask (Run asks only when modified).

	h.typeText(" Madam")
	h.press("S", key.ModShortcut)
	if got := fileText(t, path); got != "Dear Madam" || d.modified() {
		t.Errorf("Ctrl+S: %q, modified %v", got, d.modified())
	}
	clock.Advance(autosave.Delay)
	h.settle()
	if d.as.saver.Busy() || !d.as.saver.On() || d.errMsg != "" {
		t.Errorf("after Ctrl+S: busy %v, on %v, %q", d.as.saver.Busy(), d.as.saver.On(), d.errMsg)
	}

	h.clickAutoSave() // off
	if d.as.saver.On() {
		t.Fatal("not turned off")
	}
	h.typeText("!")
	clock.Advance(time.Minute)
	h.settle()
	if got := fileText(t, path); got != "Dear Madam" || !d.modified() {
		t.Errorf("off: %q", got)
	}
}

// TestAutoSaveError: a save that fails turns AutoSave off with the error
// in the status bar; the edits stay.
func TestAutoSaveError(t *testing.T) {
	h := newHarness(t)
	d := h.d
	dir := t.TempDir()
	clock := h.startAutoSave(filepath.Join(dir, "Letter.docx"))
	d.setAutoSave(true)
	h.typeText("Dear")
	os.RemoveAll(dir)
	clock.Advance(autosave.Delay)
	h.settle()
	if d.as.saver.On() || !strings.HasPrefix(d.errMsg, "AutoSave turned off: Save failed") || !d.modified() || h.text() != "Dear" {
		t.Errorf("on %v, error %q, modified %v", d.as.saver.On(), d.errMsg, d.modified())
	}
}

// TestAutoSaveDefault: with the default on, a .docx saved as starts with
// AutoSave on, a .doc with it off.
func TestAutoSaveDefault(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	old := autorecover.SettingsPath
	autorecover.SettingsPath = func() string { return settings }
	t.Cleanup(func() { autosave.SetDefault(false); autorecover.SettingsPath = old })
	h := newHarness(t)
	d := h.d
	h.startAutoSave("")
	autosave.SetDefault(true)
	dir := t.TempDir()
	d.savedAs(fileio.DOC, filepath.Join(dir, "Letter.doc"), false, d.ed.Version, nil)
	if d.as.saver.On() {
		t.Error("on for a .doc")
	}
	d.savedAs(fileio.DOCX, filepath.Join(dir, "Letter.docx"), false, d.ed.Version, nil)
	if !d.as.saver.On() {
		t.Error("off for a .docx saved as")
	}
	d.savedAs(fileio.DOC, filepath.Join(dir, "Letter.doc"), false, d.ed.Version, nil)
	h.frame()
	if d.as.saver.On() {
		t.Error("still on after saving as .doc")
	}
}
