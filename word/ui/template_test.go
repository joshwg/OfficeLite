// Tests for templates: new documents from them, editing them, the New
// page's tiles, where Save As puts them, and the window-replacement races
// around Save As.

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/common/recent"
	"officelite/common/templates"
	"officelite/word/fileio"
	"officelite/word/model"
)

// templateEnv points the home and config folders at a temporary one, so
// the templates folder, recent files and open-file registry the tests
// touch are their own, and returns it.
func templateEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, ".run"))
	return home
}

// templateWindow is a window with its own recent list and file registry,
// and the New page listing dir's templates; launched records the windows
// it asked for ("doc:", "xls:" or "tpl:" and the path).
func templateWindow(t *testing.T, dir string) (d *window, launched *[]string) {
	t.Helper()
	launched = new([]string)
	fonts := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	d = newWindow(nil, Config{Fonts: fonts, Recent: recent.Open(""),
		Launch: func(k doctype.Kind, path string) {
			prefix := "doc:"
			if k == doctype.Spreadsheet {
				prefix = "xls:"
			}
			*launched = append(*launched, prefix+path)
		},
		LaunchTemplate: func(path string) { *launched = append(*launched, "tpl:"+path) },
	})
	d.bs.tmpl.Dir = dir
	d.lk.doc = lockfile.NewDoc(func() {})
	t.Cleanup(d.lk.doc.Close)
	d.newBlank()
	waitLoaded(t, d)
	return d, launched
}

// waitLoaded runs the window's queued calls and loaded results until it
// has finished loading.
func waitLoaded(t *testing.T, d *window) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for d.loading.Load() {
		if time.Now().After(deadline) {
			t.Fatal("document didn't load")
		}
		d.calls.Run()
		d.takeResult()
		time.Sleep(time.Millisecond)
	}
	d.calls.Run()
}

// saveTemplate writes a small letterhead template in format f to dir and
// returns its path.
func saveTemplate(t *testing.T, dir string, f fileio.Format) string {
	t.Helper()
	doc := model.New("Letter")
	doc.Body = []model.Block{
		&model.Paragraph{Style: "Heading1", Runs: []model.Run{{Text: "Company Letterhead"}}},
		&model.Paragraph{Runs: []model.Run{{Text: "Dear customer,"}}},
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Letter"+f.Ext())
	if err := fileio.SaveFile(doc, path, f); err != nil {
		t.Fatal(err)
	}
	return path
}

// untitled matches the names newDocName gives.
var untitled = regexp.MustCompile(`^Document\d+$`)

// TestNewFromTemplate starts a document from a .dotx and a .dot: it is
// untitled, unmodified, holds the template's text, saves (via Save As) in
// the format the template is built on, and leaves the template file
// unregistered and out of the recent files.
func TestNewFromTemplate(t *testing.T) {
	templateEnv(t)
	for _, f := range []fileio.Format{fileio.DOTX, fileio.DOT} {
		t.Run(f.Ext(), func(t *testing.T) {
			tpl := saveTemplate(t, t.TempDir(), f)
			d, _ := templateWindow(t, "")
			d.newFromTemplate(tpl)
			waitLoaded(t, d)
			if d.errMsg != "" {
				t.Fatal(d.errMsg)
			}
			if d.path != "" || !untitled.MatchString(d.doc.Name) {
				t.Errorf("path %q, name %q: want an untitled document", d.path, d.doc.Name)
			}
			if d.modified() {
				t.Error("a new document from a template starts modified")
			}
			if p, ok := d.doc.Body[0].(*model.Paragraph); !ok || p.Text() != "Company Letterhead" || p.Style != "Heading1" {
				t.Errorf("first paragraph %+v", d.doc.Body[0])
			}
			if _, ok := d.saveFormat(); ok {
				t.Error("Save would overwrite a file; want Save As")
			}
			if got, want := saveFormats[d.bs.saveFmt], f.Document(); got != want {
				t.Errorf("Save As format %v, want %v", got, want)
			}
			if got := d.cfg.Recent.Entries(); len(got) != 0 {
				t.Errorf("recent files: %v", got)
			}
			if got := d.lk.doc.Path(); got != "" {
				t.Errorf("registered %q as the window's file", got)
			}
			if !strings.Contains(d.title, d.doc.Name) || strings.Contains(d.title, "Letter") {
				t.Errorf("title %q", d.title)
			}
		})
	}
}

// TestNewFromMissingTemplate reports a template that can't be read by its
// name and leaves the window untitled.
func TestNewFromMissingTemplate(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, "")
	d.newFromTemplate(filepath.Join(t.TempDir(), "Gone.dotx"))
	waitLoaded(t, d)
	if !strings.Contains(d.errMsg, "Gone.dotx") {
		t.Errorf("error %q doesn't name the template", d.errMsg)
	}
	if d.path != "" {
		t.Errorf("path %q", d.path)
	}
}

// TestEditTemplate opens a template the way the Open dialog and recent
// files do: the template itself is edited, Save writes it back as a
// template, and it joins the recent files.
func TestEditTemplate(t *testing.T) {
	templateEnv(t)
	for _, f := range []fileio.Format{fileio.DOTX, fileio.DOT} {
		t.Run(f.Ext(), func(t *testing.T) {
			tpl := saveTemplate(t, t.TempDir(), f)
			d, launched := templateWindow(t, "")
			d.openFile(tpl)
			waitLoaded(t, d)
			if d.errMsg != "" {
				t.Fatal(d.errMsg)
			}
			if len(*launched) != 0 {
				t.Fatalf("launched %v; want the template opened in this window", *launched)
			}
			if d.path != tpl || d.doc.Name != filepath.Base(tpl) {
				t.Errorf("path %q, name %q", d.path, d.doc.Name)
			}
			if got, ok := d.saveFormat(); !ok || got != f {
				t.Errorf("Save format %v, %v; want %v", got, ok, f)
			}
			if got := d.cfg.Recent.Entries(); len(got) != 1 || got[0].Path != tpl {
				t.Errorf("recent files: %v", got)
			}
			if want, _ := filepath.Abs(tpl); d.lk.doc.Path() != want {
				t.Errorf("registered %q, want %q", d.lk.doc.Path(), want)
			}
		})
	}
}

// TestDotmSavesAs checks that a macro-enabled template, like a .docm, has
// no Save format (WordLite doesn't write macros) and goes to Save As.
func TestDotmSavesAs(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, "")
	d.path = filepath.Join(t.TempDir(), "Macros.dotm")
	if f, ok := d.saveFormat(); ok {
		t.Errorf("Save format %v for a .dotm", f)
	}
}

// TestNewTiles lists the templates folder after the blank tiles, and each
// tile starts the right thing: blank windows through Launch, a template in
// this window while it's untouched and in a new one once it isn't.
func TestNewTiles(t *testing.T) {
	templateEnv(t)
	dir := t.TempDir()
	tpl := saveTemplate(t, dir, fileio.DOTX)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, launched := templateWindow(t, dir)

	var tiles []gui.Template
	deadline := time.Now().Add(5 * time.Second)
	for tiles = d.newTiles(); len(tiles) < len(blankTiles)+1; tiles = d.newTiles() {
		if time.Now().After(deadline) {
			t.Fatalf("templates never listed: %v", tiles)
		}
		d.calls.Run()
		time.Sleep(time.Millisecond)
	}
	if len(tiles) != len(blankTiles)+1 {
		t.Fatalf("tiles %v", tiles)
	}
	if got := tiles[len(blankTiles)]; got.Name != "Letter" || got.Path != tpl || got.Kind != recent.Document {
		t.Fatalf("template tile %+v", got)
	}

	// Blank tiles launch new windows.
	d.newFromTile(tiles[0])
	d.newFromTile(tiles[1])
	if want := []string{"doc:", "xls:"}; strings.Join(*launched, ",") != strings.Join(want, ",") {
		t.Fatalf("launched %v, want %v", *launched, want)
	}
	*launched = nil

	// An untouched window takes the new document itself…
	d.newFromTile(tiles[len(blankTiles)])
	waitLoaded(t, d)
	if len(*launched) != 0 || d.doc.Body[0].(*model.Paragraph).Text() != "Company Letterhead" {
		t.Fatalf("launched %v; first paragraph %q", *launched, d.doc.Body[0].(*model.Paragraph).Text())
	}
	// …which, once edited, goes to a new window instead.
	d.saved = -1
	d.newFromTile(tiles[len(blankTiles)])
	if want := "tpl:" + tpl; len(*launched) != 1 || (*launched)[0] != want {
		t.Errorf("launched %v, want [%s]", *launched, want)
	}

	// Out-of-range tile events are ignored.
	d.handleBackstage(gui.BackstageEvent{Action: gui.ActNew, Template: len(tiles) + 5})
	d.handleBackstage(gui.BackstageEvent{Action: gui.ActNew, Template: -1})
}

// TestTemplateSaveName suggests the templates folder for Save As
// Template, creating it.
func TestTemplateSaveName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Documents is a Known Folder on Windows")
	}
	home := templateEnv(t)
	dir := filepath.Join(home, "Documents", templates.FolderName)
	if got, want := templateSaveName("Letter.dotx"), filepath.Join(dir, "Letter.dotx"); got != want {
		t.Errorf("templateSaveName = %q, want %q", got, want)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("templates folder not created: %v", err)
	}
	// A recovered document's folder gives way to the templates folder.
	if got := templateSaveName(filepath.Join(home, "elsewhere", "Letter.dot")); got != filepath.Join(dir, "Letter.dot") {
		t.Errorf("templateSaveName = %q", got)
	}
	// Where the folder can't be made, the bare name is suggested.
	if err := os.RemoveAll(filepath.Join(home, "Documents")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "Documents"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := templateSaveName("Letter.dotx"); got != "Letter.dotx" {
		t.Errorf("templateSaveName = %q, want the bare name", got)
	}
}

// TestNotPristineWhileSaving sends a template to a new window while a
// Save As runs, rather than replacing the document being saved (whose
// path and name the save would then give the template's document).
func TestNotPristineWhileSaving(t *testing.T) {
	templateEnv(t)
	dir := t.TempDir()
	tpl := saveTemplate(t, dir, fileio.DOTX)
	d, launched := templateWindow(t, dir)
	d.saving.Store(true)
	doc := d.doc
	d.newFromTile(gui.Template{Name: "Letter", Kind: recent.Document, Path: tpl})
	if d.doc != doc || d.loading.Load() {
		t.Fatal("the document being saved was replaced")
	}
	if want := "tpl:" + tpl; len(*launched) != 1 || (*launched)[0] != want {
		t.Errorf("launched %v, want [%s]", *launched, want)
	}
}

// TestNewBlankTyping types into a new document, and picks a font, while
// its fonts load: the loader must not read the document (go test -race),
// and the font picked must stay loaded.
func TestNewBlankTyping(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, t.TempDir())
	d.newBlank()
	d.ed.InsertText("typed at once")
	d.ensureFont("Georgia")
	waitLoaded(t, d)
	if got := d.ed.Para(0).Text(); got != "typed at once" {
		t.Errorf("text %q", got)
	}
	if !slices.Contains(d.fams, "Georgia") {
		t.Errorf("the font picked while loading was dropped: %v", d.fams)
	}
}

// TestSaveAsOfReplacedDocument finishes a Save As after the window's
// document was replaced while it was written: the new document keeps its
// untitled name and no path, and the save no longer keeps the window busy.
func TestSaveAsOfReplacedDocument(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, t.TempDir())
	f, err := os.Create(filepath.Join(t.TempDir(), "Saved.docx"))
	if err != nil {
		t.Fatal(err)
	}
	gate := &gatedFile{File: f, release: make(chan struct{})}
	d.saving.Store(true)
	d.saveAsJob(fileio.DOCX, false).WriteTo(gate, f.Name())
	d.newBlank() // stands in for any load replacing the document
	waitLoaded(t, d)
	name := d.doc.Name
	close(gate.release)
	for end := time.Now().Add(5 * time.Second); d.saving.Load() && time.Now().Before(end); {
		d.calls.Run()
		time.Sleep(time.Millisecond)
	}
	if d.saving.Load() {
		t.Error("still saving")
	}
	if d.path != "" || d.doc.Name != name {
		t.Errorf("the replacing document took the save: path %q, name %q", d.path, d.doc.Name)
	}
}

// gatedFile is a file whose first write waits for release.
type gatedFile struct {
	*os.File
	release chan struct{}
	once    sync.Once
}

// Write waits for release once, then writes p to the file.
func (g *gatedFile) Write(p []byte) (int, error) {
	g.once.Do(func() { <-g.release })
	return g.File.Write(p)
}

// TestExportAbandonsClose checks that a close waiting for Save As (Save in
// the "save changes?" prompt of an untitled document) is abandoned by an
// export or by leaving for New, Open or a recent file, so a later Save As
// doesn't close the window.
func TestExportAbandonsClose(t *testing.T) {
	templateEnv(t)
	d, _ := templateWindow(t, "")
	for _, abandon := range []func(){
		func() {
			d.savedAs(saveSnap{f: fileio.TXT, export: true, version: 0}, gui.SaveAsResult{Path: filepath.Join(t.TempDir(), "x.txt")})
		},
		func() { d.handleBackstage(gui.BackstageEvent{Action: gui.ActNew, Template: 0}) },
		func() { d.handleBackstage(gui.BackstageEvent{Action: gui.ActBrowse}) },
	} {
		d.closeAfterSave = true
		abandon()
		if d.closeAfterSave {
			t.Error("close still waiting after leaving the Save As page")
		}
	}
}
