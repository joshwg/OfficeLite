// Package templates finds the user's document templates: the files in the
// Custom Office Templates folder (where Word and Excel keep personal
// templates) that the New page offers as tiles.
package templates

import (
	"errors"
	"io/fs"
	"officelite/common/limits"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// FolderName is the templates folder's name inside the Documents folder.
const FolderName = "Custom Office Templates"

// Entry is one template file.
type Entry struct {
	Path string // absolute path of the template
	Name string // file name without its extension, for the tile
}

// Dir is the user's templates folder (Documents/Custom Office Templates);
// "" when there is no Documents folder to put it in. The folder may not
// exist yet.
func Dir() string {
	docs := documentsDir()
	if docs == "" {
		return ""
	}
	return filepath.Join(docs, FolderName)
}

// List returns the files in dir whose extension is one of exts (each
// with its dot, matched without regard to case), sorted by name. Hidden
// files, Office's "~$" owner files and subfolders are skipped; a missing
// dir is an empty list, not an error.
func List(dir string, exts ...string) ([]Entry, error) {
	if dir == "" {
		return nil, nil
	}
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, de := range des {
		name := de.Name()
		if de.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
			continue
		}
		ext := filepath.Ext(name)
		if !slices.ContainsFunc(exts, func(e string) bool { return strings.EqualFold(e, ext) }) {
			continue
		}
		out = append(out, Entry{Path: filepath.Join(dir, name), Name: strings.TrimSuffix(name, ext)})
	}
	slices.SortFunc(out, func(a, b Entry) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

// Lister keeps a list of templates fresh for a UI that asks for it every
// frame: Entries returns the last list read and, when it is stale, reads
// the folder again in the background. Its methods must be called from one
// goroutine (the UI's); Post must run the function it is given there.
type Lister struct {
	Dir  string       // folder to list
	Exts []string     // extensions to list, e.g. ".dotx"
	Post func(func()) // runs a function on the UI goroutine

	entries []Entry
	read    time.Time // when the last read started
	busy    bool      // a read is running
}

// Entries returns the templates found by the last read, starting a new
// read when that one is older than limits.TemplatesRefreshEvery.
func (l *Lister) Entries() []Entry {
	if !l.busy && time.Since(l.read) >= limits.TemplatesRefreshEvery {
		l.busy, l.read = true, time.Now()
		dir, exts := l.Dir, slices.Clone(l.Exts)
		go func() {
			list, err := List(dir, exts...)
			l.Post(func() {
				l.busy = false
				if err == nil {
					l.entries = list
				}
			})
		}()
	}
	return l.entries
}
