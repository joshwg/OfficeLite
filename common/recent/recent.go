// Package recent keeps the most-recently-used file list shared by every
// OfficeLite window. It is stored as JSON in the user's config directory.
package recent

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"officelite/common/atomicfile"
)

// Max is how many files the list keeps.
const Max = 50

// Kind tells which app a file belongs to.
type Kind string

const (
	Spreadsheet Kind = "spreadsheet"
	Document    Kind = "document"
)

// Entry is one recently opened file.
type Entry struct {
	Path   string    `json:"path"`
	Kind   Kind      `json:"kind"`
	Opened time.Time `json:"opened"`
}

// Name is the file name without its directory.
func (e Entry) Name() string { return filepath.Base(e.Path) }

// Dir is the directory holding the file.
func (e Entry) Dir() string { return filepath.Dir(e.Path) }

// List is a recent-files list backed by a file. Its methods are safe for
// concurrent use by several windows.
type List struct {
	mu   sync.Mutex
	path string // "" keeps the list in memory only
	list []Entry
	rev  int
}

var (
	defaultOnce sync.Once
	defaultList *List
)

// Default is the list stored in the user's config directory.
func Default() *List {
	defaultOnce.Do(func() {
		path := ""
		if dir, err := os.UserConfigDir(); err == nil {
			path = filepath.Join(dir, "OfficeLite", "recent.json")
		}
		defaultList = Open(path)
	})
	return defaultList
}

// Open loads the list stored at path; a missing or unreadable file gives
// an empty list.
func Open(path string) *List {
	l := &List{path: path}
	if path != "" {
		l.list, _ = load(path)
	}
	return l
}

func load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []Entry
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Entries returns the list, most recent first.
func (l *List) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.list)
}

// Revision changes whenever the list does, so callers can cache.
func (l *List) Revision() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rev
}

// Add moves path to the top of the list. The list is saved at once; the
// error is from saving it (the list in memory is updated regardless).
func (l *List) Add(path string, kind Kind) error {
	if path == "" {
		return nil
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return l.update(func(list []Entry) []Entry {
		list = slices.DeleteFunc(list, func(e Entry) bool { return samePath(e.Path, path) })
		list = slices.Insert(list, 0, Entry{Path: path, Kind: kind, Opened: time.Now()})
		if len(list) > Max {
			list = list[:Max]
		}
		return list
	})
}

// Remove drops path from the list and saves it, as Add does.
func (l *List) Remove(path string) error {
	return l.update(func(list []Entry) []Entry {
		return slices.DeleteFunc(list, func(e Entry) bool { return samePath(e.Path, path) })
	})
}

// update applies change to the list and saves it. Other OfficeLite
// processes share the file, so change is applied to the list as last
// saved (by any of them), not to this process's copy, which would drop
// what the others added since.
func (l *List) update(change func([]Entry) []Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.path != "" {
		if list, err := load(l.path); err == nil {
			l.list = list
		} else if errors.Is(err, fs.ErrNotExist) {
			l.list = nil
		} // unreadable or damaged: keep this process's copy
	}
	l.list = change(l.list)
	l.rev++
	if l.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(l.list, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(l.path, data, 0o644)
}

// samePath compares paths case-insensitively on Windows-style paths.
func samePath(a, b string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}
