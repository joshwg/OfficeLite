// Package autorecover keeps recovery copies of unsaved documents, as
// Office's AutoRecover does. Every few minutes each window whose document
// has unsaved changes writes a full-fidelity copy (.xlsx or .docx, whatever
// the document's own format) and a small JSON sidecar to a per-user
// Recovery directory. Saving or closing the window deletes them; after a
// crash or power loss they are left behind, and the next OfficeLite lists
// them (Orphans) in its Document Recovery window.
package autorecover

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"officelite/common/atomicfile"
)

// Apps whose documents are recovered.
const (
	Excel = "excel"
	Word  = "word"
)

// Entry is a recovery copy's sidecar: what the copy is of and who wrote
// it.
type Entry struct {
	Path   string    `json:"path"`   // the document's file; "" if never saved
	Name   string    `json:"name"`   // the name shown, e.g. "Budget.xls" or "Book1"
	App    string    `json:"app"`    // Excel or Word
	Format string    `json:"format"` // the document's own extension, e.g. ".xls"; "" if new
	Saved  time.Time `json:"saved"`  // when the copy was written
	PID    int       `json:"pid"`    // the process keeping it; 0 for none
	// Started tells a reused pid from the process that wrote the copy
	// (Windows' process creation time; 0 where unknown).
	Started int64 `json:"started,omitempty"`

	Copy    string `json:"-"` // the recovery copy (.xlsx or .docx)
	Sidecar string `json:"-"` // this entry's file
}

// Ext is the recovery copy's extension for app.
func Ext(app string) string {
	if app == Word {
		return ".docx"
	}
	return ".xlsx"
}

// Dir is the default Recovery directory: %LOCALAPPDATA%\OfficeLite\Recovery
// on Windows, ~/.cache/OfficeLite/Recovery on Linux.
func Dir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "OfficeLite", "Recovery")
}

// Snapshot is a window's document as it is when a timer tick finds it
// changed. It is taken on the UI goroutine, so it must be cheap (a clone
// of the model); Write runs in the background.
type Snapshot struct {
	Rev    int    // changes with every edit; a copy of the same Rev isn't written again
	Path   string // as in Entry
	Name   string
	Format string
	Write  func(w io.Writer) error // encodes the clone as .xlsx / .docx
}

// Saver keeps one window's recovery copy. Its methods other than Wait run
// on the window's UI goroutine; the files are written and deleted, in
// order, on a goroutine of its own, so neither typing nor a Save or close
// waits for them. A nil Saver does nothing.
type Saver struct {
	App string
	Dir string // "" until Start: Dir()
	// Interval is how often the document is checked; nil reads the
	// user's setting (Minutes) each time. 0 turns the timer off.
	Interval func() time.Duration

	post     func(func())     // runs a function on the UI goroutine
	snapshot func() *Snapshot // nil when there's nothing to recover

	// UI goroutine only.
	base    string // file name of the copy without extension; "" until first written
	written bool   // a copy is (being) written
	rev     int    // Rev of the last copy written
	closed  bool
	busy    atomic.Bool // a write is queued or running
	queued  atomic.Bool // a tick is posted to the UI goroutine
	failed  atomic.Bool // the last write failed: try again even if unchanged
	done    chan struct{}
	started bool

	mu      sync.Mutex
	jobs    []func()
	running bool
}

// NewSaver makes the saver of a window of app. post runs a function on the
// window's UI goroutine (and wakes it); snapshot returns the document
// when it has unsaved changes, nil when it hasn't.
func NewSaver(app string, post func(func()), snapshot func() *Snapshot) *Saver {
	return &Saver{App: app, post: post, snapshot: snapshot, done: make(chan struct{})}
}

// checkEvery is how often the timer looks at the interval setting, so
// that a change to it takes effect soon.
const checkEvery = 30 * time.Second

// Start starts the timer: every interval it asks the UI goroutine (post)
// to Tick.
func (s *Saver) Start() {
	if s == nil || s.started {
		return
	}
	s.started = true
	if s.Dir == "" {
		s.Dir = Dir()
	}
	interval := s.Interval
	if interval == nil {
		interval = UserInterval
	}
	go func() {
		last := time.Now()
		for {
			wait := checkEvery
			if iv := interval(); iv > 0 {
				wait = min(max(iv-time.Since(last), time.Millisecond), checkEvery)
			}
			select {
			case <-s.done:
				return
			case <-time.After(wait):
			}
			// A tick still waiting for a busy UI goroutine isn't doubled.
			if iv := interval(); iv > 0 && time.Since(last) >= iv && s.queued.CompareAndSwap(false, true) {
				last = time.Now()
				s.post(func() {
					s.queued.Store(false)
					s.Tick()
				})
			}
		}
	}()
}

// Tick writes a recovery copy if the document has changed since the last
// one. Only the snapshot is taken here; the copy is written in the
// background. A write still going on skips this tick.
func (s *Saver) Tick() {
	if s == nil || s.closed || s.busy.Load() || s.Dir == "" {
		return
	}
	snap := s.snapshot()
	if snap == nil {
		s.Remove() // no unsaved changes (left)
		return
	}
	if s.written && snap.Rev == s.rev && !s.failed.Load() {
		return
	}
	if s.base == "" {
		s.base = baseName(snap.Name)
	}
	s.written, s.rev = true, snap.Rev
	s.busy.Store(true)
	copyPath, side := s.paths()
	e := Entry{Path: snap.Path, Name: snap.Name, App: s.App, Format: snap.Format, PID: os.Getpid(), Started: selfStart}
	s.enqueue(func() {
		defer s.busy.Store(false)
		var buf bytes.Buffer
		err := snap.Write(&buf)
		if err == nil {
			err = atomicfile.WriteFile(copyPath, buf.Bytes(), 0o600)
		}
		if err == nil {
			e.Saved = time.Now()
			err = writeEntry(side, e)
		}
		s.failed.Store(err != nil)
	})
}

// Remove deletes the window's recovery copy: the document was saved, or
// its changes are being thrown away.
func (s *Saver) Remove() {
	if s == nil || !s.written {
		return
	}
	s.written = false
	copyPath, side := s.paths()
	s.enqueue(func() {
		os.Remove(side)
		os.Remove(copyPath)
	})
}

// Close deletes the copy and stops the timer, when the window closes
// normally (after a Save or Don't Save).
func (s *Saver) Close() {
	if s == nil || s.closed {
		return
	}
	s.Remove()
	s.closed = true
	close(s.done)
}

// Adopt makes a recovered copy the window's own: the window was opened on
// it, so later copies replace it and saving or closing deletes it. rev is
// the document's Rev as loaded, which needn't be written again.
func (s *Saver) Adopt(e Entry, rev int) {
	if s == nil || e.Copy == "" {
		return
	}
	s.Dir = filepath.Dir(e.Copy)
	s.base = strings.TrimSuffix(filepath.Base(e.Copy), filepath.Ext(e.Copy))
	s.written, s.rev = true, rev
	e.PID, e.Started = os.Getpid(), selfStart
	side := e.Sidecar
	s.enqueue(func() { writeEntry(side, e) })
}

// Wait waits for the saver's queued writes and deletions (tests).
func (s *Saver) Wait() {
	for {
		s.mu.Lock()
		idle := !s.running && len(s.jobs) == 0
		s.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// paths is the saver's recovery copy and sidecar file in Dir.
func (s *Saver) paths() (copyPath, sidecar string) {
	b := filepath.Join(s.Dir, s.base)
	return b + Ext(s.App), b + ".json"
}

// pending counts the jobs of every saver, for Flush.
var pending sync.WaitGroup

// enqueue runs job after the saver's earlier jobs, on a goroutine.
func (s *Saver) enqueue(job func()) {
	pending.Add(1)
	s.mu.Lock()
	s.jobs = append(s.jobs, job)
	start := !s.running
	s.running = true
	s.mu.Unlock()
	if start {
		go s.work()
	}
}

// work runs the saver's queued jobs in order until the queue is empty.
func (s *Saver) work() {
	for {
		s.mu.Lock()
		if len(s.jobs) == 0 {
			s.running = false
			s.mu.Unlock()
			return
		}
		job := s.jobs[0]
		s.jobs = s.jobs[1:]
		s.mu.Unlock()
		job()
		pending.Done()
	}
}

// Flush waits, at most timeout, for the copies being written or deleted,
// so that exiting after the last window closed leaves none behind.
func Flush(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		pending.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// baseName makes a copy's file name from the document's name: its stem,
// cleaned for any file system, and a random suffix, so every window has
// its own.
func baseName(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	stem = strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, stem)
	stem = strings.Trim(stem, " .")
	if r := []rune(stem); len(r) > 60 {
		stem = string(r[:60])
	}
	if stem == "" {
		stem = "Document"
	}
	var b [5]byte
	rand.Read(b[:])
	return stem + "~" + hex.EncodeToString(b[:])
}

// writeEntry writes e as a sidecar to path, atomically.
func writeEntry(path string, e Entry) error {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, b, 0o600)
}

// readEntry reads a sidecar and finds its copy.
func readEntry(path string) (Entry, error) {
	var e Entry
	b, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(b, &e)
	}
	e.Sidecar = path
	e.Copy = strings.TrimSuffix(path, ".json") + Ext(e.App)
	return e, err
}
