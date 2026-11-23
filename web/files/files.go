// Package files keeps users' documents on the server's disk: one folder
// per user under the root (data/users/<username in lowercase>/), each
// document an ordinary file and each of the user's folders an ordinary
// folder under its own name, so the folders can be backed up, inspected
// and restored by hand. Names and paths ("Reports/2026/Budget.xlsx")
// follow web/names and are found without regard to case. A write goes to
// a hidden temporary file in the document's folder that is renamed over
// the document, so a failed or cut-off upload never leaves half a file;
// writes, moves and deletes of one user's files take turns, so the quota
// and folder checks can't be raced.
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
	// ErrNotFound: the user has no file or folder of that name.
	ErrNotFound = errors.New("no such file or folder")
	// ErrNoFolder: the folder to put something in isn't there (deleted,
	// moved or renamed, perhaps from another tab, or never made).
	ErrNoFolder = errors.New("that folder isn't there")
	// ErrExists: a file or folder of that name (in any case) is already
	// there.
	ErrExists = errors.New("a file or folder with that name already exists")
	// ErrQuota: the write would take the user past their quota.
	ErrQuota = errors.New("not enough storage left")
	// ErrTooLarge: the file is over web.FileMaxBytes.
	ErrTooLarge = fmt.Errorf("file is over the %d MiB limit", web.FileMaxBytes>>20)
	// ErrNotEmpty: a folder to delete without its contents has some.
	ErrNotEmpty = errors.New("the folder isn't empty")
	// ErrTooManyFolders: the user already has web.FolderMaxCount folders.
	ErrTooManyFolders = fmt.Errorf("you can have at most %d folders", web.FolderMaxCount)
	// ErrTooDeep: a move would put folders more than web.FolderMaxDepth
	// deep.
	ErrTooDeep = fmt.Errorf("folders may be at most %d deep", web.FolderMaxDepth)
	// ErrIntoItself: a folder can't move into itself or a folder in it.
	ErrIntoItself = errors.New("a folder can't move into itself")
)

// NameError is a username, file name, folder name or path that breaks
// the rules in web/names.
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
	Path     string    `json:"path"` // with its folders, as stored: "Reports/Budget.xlsx"
	Size     int64     `json:"size"`
	Modified time.Time `json:"-"`
	// ModifiedMS is Modified in Unix milliseconds, for the JSON API.
	ModifiedMS int64 `json:"modified"`
}

// Folder describes one of a user's folders.
type Folder struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"` // with the folders above it, as stored
	Modified   time.Time `json:"-"`
	ModifiedMS int64     `json:"modified"` // Modified in Unix milliseconds
}

// Listing is what one of a user's folders holds.
type Listing struct {
	Dir     string   `json:"dir"`     // the folder's path as stored; "" for the top
	Folders []Folder `json:"folders"` // by name, ignoring case
	Files   []Info   `json:"files"`   // by name, ignoring case
	Used    int64    `json:"used"`    // bytes of all the user's documents, in every folder
}

// joinPath joins a stored folder path and a name with '/'.
func joinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// infoOf describes the document fi in the folder at path dir.
func infoOf(dir string, fi fs.FileInfo) Info {
	return Info{Name: fi.Name(), Path: joinPath(dir, fi.Name()), Size: fi.Size(), Modified: fi.ModTime(), ModifiedMS: fi.ModTime().UnixMilli()}
}

// folderOf describes the folder fi in the folder at path dir.
func folderOf(dir string, fi fs.FileInfo) Folder {
	return Folder{Name: fi.Name(), Path: joinPath(dir, fi.Name()), Modified: fi.ModTime(), ModifiedMS: fi.ModTime().UnixMilli()}
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

// contents reads the folders and documents in dir (on disk), described
// as at path rel. Temporary files, hidden names, folders whose names
// break the rules and anything else are skipped; a missing folder holds
// nothing.
func contents(dir, rel string) ([]Folder, []Info, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, err
	}
	var folders []Folder
	var docs []Info
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		isDir := e.IsDir()
		if isDir {
			if clean, err := names.FolderName(name); err != nil || clean != name {
				continue
			}
		} else if !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed meanwhile
		} else if err != nil {
			return nil, nil, err
		}
		if isDir {
			folders = append(folders, folderOf(rel, fi))
		} else {
			docs = append(docs, infoOf(rel, fi))
		}
	}
	return folders, docs, nil
}

// documents reads the documents in dir (on disk), described as at rel.
func documents(dir, rel string) ([]Info, error) {
	_, docs, err := contents(dir, rel)
	return docs, err
}

// findDoc returns the document in docs named name without regard to case.
func findDoc(docs []Info, name string) (Info, bool) {
	for _, d := range docs {
		if strings.EqualFold(d.Name, name) {
			return d, true
		}
	}
	return Info{}, false
}

// findFolder returns the folder in folders named name without regard to
// case.
func findFolder(folders []Folder, name string) (Folder, bool) {
	for _, f := range folders {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return Folder{}, false
}

// resolve finds the folder at parts (folder names, any case) under the
// user's folder home and returns where it is on disk and its path as
// stored, or ErrNotFound.
func resolve(home string, parts []string) (string, string, error) {
	dir, rel := home, ""
	for _, p := range parts {
		folders, _, err := contents(dir, rel)
		if err != nil {
			return "", "", err
		}
		f, ok := findFolder(folders, p)
		if !ok {
			return "", "", ErrNotFound
		}
		dir, rel = filepath.Join(dir, f.Name), f.Path
	}
	return dir, rel, nil
}

// resolveFolder is resolve for a folder something goes into: a missing
// one is ErrNoFolder.
func resolveFolder(home string, parts []string) (string, string, error) {
	dir, rel, err := resolve(home, parts)
	if errors.Is(err, ErrNotFound) {
		err = ErrNoFolder
	}
	return dir, rel, err
}

// usage returns the bytes of the documents and the number of folders in
// the user's folder home, counting every folder in it.
func usage(home string) (int64, int, error) {
	var used int64
	count := 0
	var walk func(dir string) error
	walk = func(dir string) error {
		folders, docs, err := contents(dir, "")
		if err != nil {
			return err
		}
		for _, d := range docs {
			used += d.Size
		}
		for _, f := range folders {
			count++
			if err := walk(filepath.Join(dir, f.Name)); err != nil {
				return err
			}
		}
		return nil
	}
	err := walk(home)
	return used, count, err
}

// depthBelow returns how many folders deep dir's own folders go: 0 for a
// folder with none.
func depthBelow(dir string) (int, error) {
	folders, _, err := contents(dir, "")
	if err != nil {
		return 0, err
	}
	deepest := 0
	for _, f := range folders {
		d, err := depthBelow(filepath.Join(dir, f.Name))
		if err != nil {
			return 0, err
		}
		deepest = max(deepest, d+1)
	}
	return deepest, nil
}

// filePath applies the path rules to a document's path or returns a
// *NameError.
func filePath(path string) ([]string, string, error) {
	dirs, name, err := names.FilePath(path)
	if err != nil {
		return nil, "", &NameError{err}
	}
	return dirs, name, nil
}

// folderPath applies the path rules to a folder's path ("" for the top)
// or returns a *NameError.
func folderPath(path string) ([]string, error) {
	parts, err := names.FolderPath(path)
	if err != nil {
		return nil, &NameError{err}
	}
	return parts, nil
}

// IsFolderPath reports whether path names a folder rather than a
// document: its last name doesn't end in one of names.StorableExts.
func IsFolderPath(path string) bool {
	return !names.Storable(strings.Trim(path[strings.LastIndexByte(path, '/')+1:], " "))
}

// List returns what username's folder dir ("" for the top) holds, and
// the bytes all their documents take up.
func (s *Store) List(username, dir string) (Listing, error) {
	parts, err := folderPath(dir)
	if err != nil {
		return Listing{}, err
	}
	home, err := s.Dir(username)
	if err != nil {
		return Listing{}, err
	}
	abs, rel, err := resolve(home, parts)
	if err != nil {
		return Listing{}, err
	}
	folders, docs, err := contents(abs, rel)
	if err != nil {
		return Listing{}, err
	}
	used, _, err := usage(home)
	if err != nil {
		return Listing{}, err
	}
	sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name) })
	sort.Slice(docs, func(i, j int) bool { return strings.ToLower(docs[i].Name) < strings.ToLower(docs[j].Name) })
	return Listing{Dir: rel, Folders: folders, Files: docs, Used: used}, nil
}

// Usage returns the bytes all username's documents take up.
func (s *Store) Usage(username string) (int64, error) {
	home, err := s.Dir(username)
	if err != nil {
		return 0, err
	}
	used, _, err := usage(home)
	return used, err
}

// AllFolders returns the paths of all username's folders, each after the
// folder holding it, by name ignoring case (for choosing where to move
// a file).
func (s *Store) AllFolders(username string) ([]string, error) {
	home, err := s.Dir(username)
	if err != nil {
		return nil, err
	}
	var out []string
	var walk func(dir, rel string) error
	walk = func(dir, rel string) error {
		folders, _, err := contents(dir, rel)
		if err != nil {
			return err
		}
		sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name) })
		for _, f := range folders {
			out = append(out, f.Path)
			if err := walk(filepath.Join(dir, f.Name), f.Path); err != nil {
				return err
			}
		}
		return nil
	}
	return out, walk(home, "")
}

// Stat describes username's document at path (found without regard to
// case).
func (s *Store) Stat(username, path string) (Info, error) {
	d, _, err := s.stat(username, path)
	return d, err
}

// stat is Stat that also returns where the document is on disk.
func (s *Store) stat(username, path string) (Info, string, error) {
	dirs, name, err := filePath(path)
	if err != nil {
		return Info{}, "", err
	}
	home, err := s.Dir(username)
	if err != nil {
		return Info{}, "", err
	}
	abs, rel, err := resolve(home, dirs)
	if err != nil {
		return Info{}, "", err
	}
	docs, err := documents(abs, rel)
	if err != nil {
		return Info{}, "", err
	}
	d, ok := findDoc(docs, name)
	if !ok {
		return Info{}, "", ErrNotFound
	}
	return d, filepath.Join(abs, d.Name), nil
}

// Open opens username's document at path for reading; the caller closes
// it.
func (s *Store) Open(username, path string) (*os.File, Info, error) {
	d, file, err := s.stat(username, path)
	if err != nil {
		return nil, Info{}, err
	}
	f, err := os.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, Info{}, ErrNotFound
	}
	return f, d, err
}

// Put stores r as username's document at path, whose folder must exist
// (else ErrNoFolder),
// within quota bytes for all their documents and web.FileMaxBytes for
// this one. An existing document of that name (in any case) is replaced,
// keeping its name, when replace is set, else Put fails with ErrExists.
// Nothing is changed unless the whole of r is stored.
func (s *Store) Put(username, path string, r io.Reader, quota int64, replace bool) (Info, error) {
	dirs, name, err := filePath(path)
	if err != nil {
		return Info{}, err
	}
	home, unlock, err := s.lock(username)
	if err != nil {
		return Info{}, err
	}
	defer unlock()
	if len(dirs) == 0 {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return Info{}, err
		}
	}
	dir, rel, err := resolveFolder(home, dirs)
	if err != nil {
		return Info{}, err
	}
	folders, docs, err := contents(dir, rel)
	if err != nil {
		return Info{}, err
	}
	if _, ok := findFolder(folders, name); ok {
		return Info{}, ErrExists // can't happen with the name rules, but never write over a folder
	}
	target := name
	var oldSize int64
	if old, ok := findDoc(docs, name); ok {
		if !replace {
			return Info{}, ErrExists
		}
		target, oldSize = old.Name, old.Size
	}
	used, _, err := usage(home)
	if err != nil {
		return Info{}, err
	}
	limit := min(max(quota-(used-oldSize), 0), web.FileMaxBytes)
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
	return infoOf(rel, fi), nil
}

// MakeFolder makes username's folder at path, in a folder that must
// exist (else ErrNoFolder). It fails with ErrExists when that folder already has a folder
// of the name (in any case), and with ErrTooManyFolders past
// web.FolderMaxCount.
func (s *Store) MakeFolder(username, path string) (Folder, error) {
	parts, err := folderPath(path)
	if err != nil {
		return Folder{}, err
	}
	if len(parts) == 0 {
		return Folder{}, &NameError{errors.New("folder name is required")}
	}
	home, unlock, err := s.lock(username)
	if err != nil {
		return Folder{}, err
	}
	defer unlock()
	if err := os.MkdirAll(home, 0o700); err != nil {
		return Folder{}, err
	}
	parent, rel, err := resolveFolder(home, parts[:len(parts)-1])
	if err != nil {
		return Folder{}, err
	}
	name := parts[len(parts)-1]
	folders, docs, err := contents(parent, rel)
	if err != nil {
		return Folder{}, err
	}
	_, isFolder := findFolder(folders, name)
	_, isDoc := findDoc(docs, name)
	if isFolder || isDoc {
		return Folder{}, ErrExists
	}
	if _, count, err := usage(home); err != nil {
		return Folder{}, err
	} else if count >= web.FolderMaxCount {
		return Folder{}, ErrTooManyFolders
	}
	if err := os.Mkdir(filepath.Join(parent, name), 0o700); err != nil {
		return Folder{}, err
	}
	fi, err := os.Stat(filepath.Join(parent, name))
	if err != nil {
		return Folder{}, err
	}
	return folderOf(rel, fi), nil
}

// DeleteFolder deletes username's folder at path: with everything in it
// when all is set, else only when it holds no documents or folders
// (ErrNotEmpty).
func (s *Store) DeleteFolder(username, path string, all bool) error {
	parts, err := folderPath(path)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return &NameError{errors.New("the top folder can't be deleted")}
	}
	home, unlock, err := s.lock(username)
	if err != nil {
		return err
	}
	defer unlock()
	dir, rel, err := resolve(home, parts)
	if err != nil {
		return err
	}
	if !all {
		folders, docs, err := contents(dir, rel)
		if err != nil {
			return err
		}
		if len(folders) > 0 || len(docs) > 0 {
			return ErrNotEmpty
		}
	}
	return os.RemoveAll(dir) // also any hidden leftovers in an "empty" one
}

// Move moves or renames username's document or folder from to to (paths
// whose last names say which: a document's ends in a stored extension).
// The folder to goes into must exist (else ErrNoFolder); a missing
// from is ErrNotFound. It fails with ErrExists when that
// folder already has something named like to's last name (in any case);
// changing only the case of a name is allowed. A folder can't move into
// itself (ErrIntoItself) or to where its folders would be more than
// web.FolderMaxDepth deep (ErrTooDeep). It returns the new path as
// stored.
func (s *Store) Move(username, from, to string) (string, error) {
	folder := IsFolderPath(from)
	if folder != IsFolderPath(to) {
		if folder {
			return "", &NameError{errors.New("a folder's new name may not end in a file extension")}
		}
		return "", &NameError{fmt.Errorf("a file's new name must end in one of %s", strings.Join(names.StorableExts, " "))}
	}
	var fromDirs, toDirs []string
	var fromName, toName string
	var err error
	if folder {
		if fromDirs, err = folderPath(from); err == nil {
			toDirs, err = folderPath(to)
		}
		if err == nil && (len(fromDirs) == 0 || len(toDirs) == 0) {
			err = &NameError{errors.New("folder name is required")}
		}
		if err != nil {
			return "", err
		}
		fromDirs, fromName = fromDirs[:len(fromDirs)-1], fromDirs[len(fromDirs)-1]
		toDirs, toName = toDirs[:len(toDirs)-1], toDirs[len(toDirs)-1]
	} else {
		if fromDirs, fromName, err = filePath(from); err != nil {
			return "", err
		}
		if toDirs, toName, err = filePath(to); err != nil {
			return "", err
		}
	}
	home, unlock, err := s.lock(username)
	if err != nil {
		return "", err
	}
	defer unlock()
	srcDir, srcRel, err := resolve(home, fromDirs)
	if err != nil {
		return "", err
	}
	srcFolders, srcDocs, err := contents(srcDir, srcRel)
	if err != nil {
		return "", err
	}
	var srcName, srcPath string
	if folder {
		f, ok := findFolder(srcFolders, fromName)
		if !ok {
			return "", ErrNotFound
		}
		srcName, srcPath = f.Name, f.Path
	} else {
		d, ok := findDoc(srcDocs, fromName)
		if !ok {
			return "", ErrNotFound
		}
		srcName, srcPath = d.Name, d.Path
	}
	dstDir, dstRel, err := resolveFolder(home, toDirs)
	if err != nil {
		return "", err
	}
	if folder {
		lower := strings.ToLower(srcPath)
		if d := strings.ToLower(dstRel); d == lower || strings.HasPrefix(d, lower+"/") {
			return "", ErrIntoItself
		}
		below, err := depthBelow(filepath.Join(srcDir, srcName))
		if err != nil {
			return "", err
		}
		if len(toDirs)+1+below > web.FolderMaxDepth {
			return "", ErrTooDeep
		}
	}
	dstFolders, dstDocs, err := contents(dstDir, dstRel)
	if err != nil {
		return "", err
	}
	same := srcDir == dstDir
	if f, ok := findFolder(dstFolders, toName); ok && !(same && folder && f.Name == srcName) {
		return "", ErrExists
	}
	if d, ok := findDoc(dstDocs, toName); ok && !(same && !folder && d.Name == srcName) {
		return "", ErrExists
	}
	if err := os.Rename(filepath.Join(srcDir, srcName), filepath.Join(dstDir, toName)); err != nil {
		return "", err
	}
	return joinPath(dstRel, toName), nil
}

// Delete deletes username's document at path.
func (s *Store) Delete(username, path string) error {
	dirs, name, err := filePath(path)
	if err != nil {
		return err
	}
	home, unlock, err := s.lock(username)
	if err != nil {
		return err
	}
	defer unlock()
	dir, rel, err := resolve(home, dirs)
	if err != nil {
		return err
	}
	docs, err := documents(dir, rel)
	if err != nil {
		return err
	}
	d, ok := findDoc(docs, name)
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

// recentName is the file in a user's folder holding their recent-files
// list (the apps' File → Recent). The leading '.' keeps it out of their
// documents.
const recentName = ".recent.json"

// Recent returns username's recent-files list as stored by SetRecent, or
// nil when there is none yet.
func (s *Store) Recent(username string) ([]byte, error) {
	dir, err := s.Dir(username)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, recentName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// SetRecent stores data as username's recent-files list, whole or not
// at all. It isn't counted against their quota (web.RecentMaxBytes caps
// it).
func (s *Store) SetRecent(username string, data []byte) error {
	dir, unlock, err := s.lock(username)
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, recentName))
	}
	if err != nil {
		if rmErr := os.Remove(tmp.Name()); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, rmErr)
		}
	}
	return err
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
