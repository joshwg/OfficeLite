// Package recent keeps the most-recently-used file list shared by every
// OfficeLite window. It is stored as JSON in the user's config directory.
package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
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
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, &l.list)
		}
	}
	return l
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

// Add moves path to the top of the list.
func (l *List) Add(path string, kind Kind) {
	if path == "" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = slices.DeleteFunc(l.list, func(e Entry) bool { return samePath(e.Path, path) })
	l.list = slices.Insert(l.list, 0, Entry{Path: path, Kind: kind, Opened: time.Now()})
	if len(l.list) > Max {
		l.list = l.list[:Max]
	}
	l.saveLocked()
}

// Remove drops path from the list.
func (l *List) Remove(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = slices.DeleteFunc(l.list, func(e Entry) bool { return samePath(e.Path, path) })
	l.saveLocked()
}

func (l *List) saveLocked() {
	l.rev++
	if l.path == "" {
		return
	}
	data, err := json.MarshalIndent(l.list, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(l.path), 0o755) != nil {
		return
	}
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, l.path)
	}
}

// samePath compares paths case-insensitively on Windows-style paths.
func samePath(a, b string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}
