// Package files keeps users' documents on the server's disk: one folder
// per user under the root (data/users/<username in lowercase>/), each
// document an ordinary file under its own name, so the folders can be
// backed up, inspected and restored by hand. Names follow web/names and
// are found without regard to case. A write goes to a hidden temporary
// file in the folder that is renamed over the document, so a failed or
// cut-off upload never leaves half a file; writes, renames and deletes
// of one user's files take turns, so the quota check can't be raced.
package files

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"officelite/web"
	"officelite/web/names"
)

// Errors for expected conditions; the handlers turn them into messages.
var (
	// ErrNotFound: the user has no file of that name.
	ErrNotFound = errors.New("no such file")
	// ErrExists: a file of that name (in any case) is already there.
	ErrExists = errors.New("a file with that name already exists")
	// ErrQuota: the write would take the user past their quota.
	ErrQuota = errors.New("not enough storage left")
	// ErrTooLarge: the file is over web.FileMaxBytes.
	ErrTooLarge = fmt.Errorf("file is over the %d MiB limit", web.FileMaxBytes>>20)
)

// NameError is a username or file name that breaks the rules in
// web/names.
type NameError struct{ Err error }

// Error is the rule broken.
func (e *NameError) Error() string { return e.Err.Error() }

// Unwrap returns the rule's error.
func (e *NameError) Unwrap() error { return e.Err }

// tempPrefix starts the hidden temporary files of writes in progress;
// names can't start with '.', so no document is ever taken for one.
const tempPrefix = ".upload-"

// Info describes a stored document.
type Info struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"-"`
	// ModifiedMS is Modified in Unix milliseconds, for the JSON API.
	ModifiedMS int64 `json:"modified"`
}

// infoOf describes the document fi.
func infoOf(fi fs.FileInfo) Info {
	return Info{Name: fi.Name(), Size: fi.Size(), Modified: fi.ModTime(), ModifiedMS: fi.ModTime().UnixMilli()}
}

// Store is the documents of all users under one root folder.
type Store struct {
	root  string
	mu    sync.Mutex
	locks map[string]*sync.Mutex // per user folder
}

// New returns the store rooted at root (created when first written).
func New(root string) *Store {
	return &Store{root: root, locks: map[string]*sync.Mutex{}}
}

// Root is the folder holding the users' folders.
func (s *Store) Root() string { return s.root }

// Dir is username's folder, or a *NameError for an invalid username.
func (s *Store) Dir(username string) (string, error) {
	if err := names.Username(username); err != nil {
		return "", &NameError{err}
	}
	return filepath.Join(s.root, names.UserDir(username)), nil
}

// lock takes username's turn at changing their files and returns the
// folder and the function that gives the turn back.
func (s *Store) lock(username string) (string, func(), error) {
	dir, err := s.Dir(username)
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	m := s.locks[dir]
	if m == nil {
		m = &sync.Mutex{}
		s.locks[dir] = m
	}
	s.mu.Unlock()
	m.Lock()
	return dir, m.Unlock, nil
}

// documents reads dir's documents (skipping temporary files and anything
// that isn't a regular file); a missing folder has none.
func documents(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed meanwhile
		} else if err != nil {
			return nil, err
		}
		out = append(out, infoOf(fi))
	}
	return out, nil
}

// find returns the document in docs named name without regard to case.
func find(docs []Info, name string) (Info, bool) {
	for _, d := range docs {
		if strings.EqualFold(d.Name, name) {
			return d, true
		}
	}
	return Info{}, false
}

// cleanName applies the file-name rules (trimming spaces) or returns a
// *NameError.
func cleanName(name string) (string, error) {
	clean, err := names.FileName(name)
	if err != nil {
		return "", &NameError{err}
	}
	return clean, nil
}

// List returns username's documents by name (ignoring case) and the bytes
// they take up.
func (s *Store) List(username string) ([]Info, int64, error) {
	dir, err := s.Dir(username)
	if err != nil {
		return nil, 0, err
	}
	docs, err := documents(dir)
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(docs, func(i, j int) bool { return strings.ToLower(docs[i].Name) < strings.ToLower(docs[j].Name) })
	var used int64
	for _, d := range docs {
		used += d.Size
	}
	return docs, used, nil
}

// Stat describes username's document name (found without regard to
// case).
func (s *Store) Stat(username, name string) (Info, error) {
	name, err := cleanName(name)
	if err != nil {
		return Info{}, err
	}
	dir, err := s.Dir(username)
	if err != nil {
		return Info{}, err
	}
	docs, err := documents(dir)
	if err != nil {
		return Info{}, err
	}
	d, ok := find(docs, name)
	if !ok {
		return Info{}, ErrNotFound
	}
	return d, nil
}

// Open opens username's document name for reading; the caller closes it.
func (s *Store) Open(username, name string) (*os.File, Info, error) {
	d, err := s.Stat(username, name)
	if err != nil {
		return nil, Info{}, err
	}
	dir, _ := s.Dir(username) // valid: Stat checked it
	f, err := os.Open(filepath.Join(dir, d.Name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, Info{}, ErrNotFound
	}
	return f, d, err
}

// Put stores r as username's document name, within quota bytes for all
// their documents and web.FileMaxBytes for this one. An existing document
// of that name (in any case) is replaced, keeping its name, when replace
// is set, else Put fails with ErrExists. Nothing is changed unless the
// whole of r is stored.
func (s *Store) Put(username, name string, r io.Reader, quota int64, replace bool) (Info, error) {
	name, err := cleanName(name)
	if err != nil {
		return Info{}, err
	}
	dir, unlock, err := s.lock(username)
	if err != nil {
		return Info{}, err
	}
	defer unlock()
	docs, err := documents(dir)
	if err != nil {
		return Info{}, err
	}
	target := name
	if old, ok := find(docs, name); ok {
		if !replace {
			return Info{}, ErrExists
		}
		target = old.Name
	}
	var used int64
	for _, d := range docs {
		if d.Name != target {
			used += d.Size
		}
	}
	limit := min(max(quota-used, 0), web.FileMaxBytes)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, err
	}
	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return Info{}, err
	}
	n, err := io.Copy(tmp, io.LimitReader(r, limit+1))
	switch {
	case err != nil:
	case n > limit && limit == web.FileMaxBytes:
		err = ErrTooLarge
	case n > limit:
		err = ErrQuota
	default:
		// On disk before the rename, so a crash can't leave the
		// document renamed to a partly written file.
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, target))
	}
	if err != nil {
		if rmErr := os.Remove(tmp.Name()); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, rmErr)
		}
		return Info{}, err
	}
	fi, err := os.Stat(filepath.Join(dir, target))
	if err != nil {
		return Info{}, err
	}
	return infoOf(fi), nil
}

// Rename renames username's document from to to. It fails with
// ErrExists when another document already has the name to (in any
// case); changing only the case of a name is allowed.
func (s *Store) Rename(username, from, to string) (Info, error) {
	from, err := cleanName(from)
	if err != nil {
		return Info{}, err
	}
	to, err = cleanName(to)
	if err != nil {
		return Info{}, err
	}
	dir, unlock, err := s.lock(username)
	if err != nil {
		return Info{}, err
	}
	defer unlock()
	docs, err := documents(dir)
	if err != nil {
		return Info{}, err
	}
	src, ok := find(docs, from)
	if !ok {
		return Info{}, ErrNotFound
	}
	if dst, ok := find(docs, to); ok && dst.Name != src.Name {
		return Info{}, ErrExists
	}
	if err := os.Rename(filepath.Join(dir, src.Name), filepath.Join(dir, to)); err != nil {
		return Info{}, err
	}
	fi, err := os.Stat(filepath.Join(dir, to))
	if err != nil {
		return Info{}, err
	}
	return infoOf(fi), nil
}

// Delete deletes username's document name.
func (s *Store) Delete(username, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	dir, unlock, err := s.lock(username)
	if err != nil {
		return err
	}
	defer unlock()
	docs, err := documents(dir)
	if err != nil {
		return err
	}
	d, ok := find(docs, name)
	if !ok {
		return ErrNotFound
	}
	return os.Remove(filepath.Join(dir, d.Name))
}

// HasFolder reports whether username's folder exists (left over, for
// example, from a deleted account whose folder couldn't be removed).
func (s *Store) HasFolder(username string) (bool, error) {
	dir, err := s.Dir(username)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(dir)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// RemoveUser deletes username's folder and every file in it.
func (s *Store) RemoveUser(username string) error {
	dir, unlock, err := s.lock(username)
	if err != nil {
		return err
	}
	defer unlock()
	return os.RemoveAll(dir)
}
