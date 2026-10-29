package lockfile

import (
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// now is the clock, for tests.
var now = time.Now

// Status is what Check found about a document.
type Status struct {
	// Owner is the user named in another program's live owner file.
	Owner string
	// InUse: another program (Word, Excel) holds the file open against
	// writing.
	InUse bool
	// Elsewhere: another OfficeLite process has the file open and could
	// not be asked to show it.
	Elsewhere bool
}

// Locked reports whether opening the file should ask first.
func (s Status) Locked() bool { return s.Owner != "" || s.InUse || s.Elsewhere }

// Title is the lock dialog's title, as Office's.
func (s Status) Title() string { return "File In Use" }

// Message is the lock dialog's text for the file called name.
func (s Status) Message(name string) string {
	const choice = "\n\nOpen a read-only copy, or open it anyway (saving may overwrite their changes)."
	switch {
	case s.Elsewhere:
		return name + " is already open in another OfficeLite window."
	case s.Owner != "":
		return name + " is locked for editing by " + s.Owner + "." + choice
	}
	return name + " is in use by another program (Word or Excel?)." + choice
}

// Buttons are the lock dialog's buttons: Open Read-Only, Open Anyway
// (not for a file open in another OfficeLite window), Cancel.
func (s Status) Buttons() []string {
	if s.Elsewhere {
		return []string{"Open Read-Only", "Cancel"}
	}
	return []string{"Open Read-Only", "Open Anyway", "Cancel"}
}

// Choice is an answer to the lock dialog.
type Choice int

const (
	ReadOnly Choice = iota
	Anyway
	Cancel
)

// Choice is the answer the dialog's button (an index into Buttons; -1 for
// Esc) gives.
func (s Status) Choice(button int) Choice {
	switch n := len(s.Buttons()); {
	case button == 0:
		return ReadOnly
	case button < 0 || button >= n-1:
		return Cancel
	}
	return Anyway
}

// Check looks for signs that the document at path is open elsewhere: a
// live owner file next to it, a sharing lock on it, or another OfficeLite
// process having it open. A stale owner file naming this user (left by a
// crash) is deleted; other programs' stale ones are left alone.
func Check(path string) Status {
	var st Status
	me := UserName()
	for _, op := range ownerPaths(path) {
		if _, err := os.Lstat(op); err != nil || holds(op) {
			continue
		}
		name, ok := Decode(readOwner(op))
		if !ownerLive(op) {
			if ok && name == me {
				os.Remove(op)
			}
			continue
		}
		if !ok {
			name = "another user"
		}
		st.Owner = name
		break
	}
	st.InUse = inUse(path)
	st.Elsewhere = openElsewhere(path)
	return st
}

// InUse reports whether another program holds the file open against
// writing (always false outside Windows).
func InUse(path string) bool { return inUse(path) }

// SaveError is the message for a failed save of path: a plain one, or,
// when another program has the file open, how to keep the changes.
func SaveError(path string, err error) string {
	if sharingViolation(err) || inUse(path) {
		return filepath.Base(path) + " is open in another program (Word or Excel?) — save a copy with Save As"
	}
	return "Save failed: " + err.Error()
}

// readOwner reads the head of an owner file; nil if it can't be read
// (Office may share it for reading only, or not at all).
func readOwner(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 512))
	return b
}

var (
	userOnce sync.Once
	userName string
)

// UserName is the name OfficeLite writes in owner files: the Windows
// display name where there is one, else the account's full name or login.
func UserName() string {
	userOnce.Do(func() {
		if n, err := displayName(); err == nil {
			userName = n
			return
		}
		if u, err := user.Current(); err == nil {
			userName = u.Name
			if userName == "" {
				userName = u.Username[strings.LastIndex(u.Username, `\`)+1:]
			}
		}
		if userName == "" {
			userName = "OfficeLite user"
		}
	})
	return userName
}

// owner is an owner file this process holds for a document.
type owner struct {
	path string
	f    *os.File
}

var held sync.Map // key(owner path) → true

func holds(path string) bool {
	_, ok := held.Load(key(path))
	return ok
}

// acquireOwner writes an owner file for the document at path when it is
// on a network share outside the cloud-sync folders, where Office users
// would open it too, and keeps it open. It returns nil (silently) when
// there is none to write, someone else's is there, or it can't be
// created (a read-only share).
func acquireOwner(path string) *owner {
	f, ok := FormatOf(path)
	if !ok || cloudFolder(path) || !isNetwork(path) {
		return nil
	}
	op := filepath.Join(filepath.Dir(path), OwnerName(filepath.Base(path)))
	if holds(op) {
		return nil // another window's (a document sharing the name)
	}
	if _, err := os.Lstat(op); err == nil {
		// Replace only this user's own leftover.
		if name, ok := Decode(readOwner(op)); !ok || name != UserName() || ownerLive(op) {
			return nil
		}
	}
	file, err := createOwner(op, Encode(f, UserName()))
	if err != nil {
		return nil
	}
	held.Store(key(op), true)
	return &owner{path: op, f: file}
}

// release closes and deletes the owner file.
func (o *owner) release() {
	if o == nil {
		return
	}
	held.Delete(key(o.path))
	o.f.Close()
	os.Remove(o.path)
}

// cloudFolder reports whether path is inside a OneDrive, Dropbox, Google
// Drive or iCloud folder, which sync rather than share: Office doesn't
// lock files there with owner files.
func cloudFolder(path string) bool {
	for _, env := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
		if d := os.Getenv(env); d != "" && withinDir(path, d) {
			return true
		}
	}
	for _, part := range strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '/' || r == '\\' }) {
		switch {
		case strings.HasPrefix(part, "onedrive"), part == "dropbox", part == "google drive", part == "my drive",
			part == "icloud drive", part == "iclouddrive":
			return true
		}
	}
	return false
}

func withinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
