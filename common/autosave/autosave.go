// Package autosave saves a document into its own file as it is edited,
// as Microsoft 365's AutoSave does for files in OneDrive, but locally:
// a few seconds after the last edit, and at least every minute while the
// edits keep coming. Each window has a Saver; the window takes a snapshot
// (a clone of its model) on the UI goroutine and the file is written in
// the background, through the same atomic save as Ctrl+S.
//
// There is no version history behind it, so AutoSave is off unless the
// user turns it on (per document, or by default in the settings file).
package autosave

import (
	"sync"
	"time"

	"officelite/common/autorecover"
)

// Delay is how long after the last edit the document is saved.
const Delay = 3 * time.Second

// MaxWait is the longest edits wait while more keep coming.
const MaxWait = 60 * time.Second

// Clock is a Saver's time: the real one, or a fake in tests.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f on a goroutine of its own after d; stop cancels
	// it.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}

// ManualClock is a Clock that moves only when told to (tests).
type ManualClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualTimer
}

type manualTimer struct {
	at      time.Time
	f       func()
	stopped bool
}

// NewManualClock starts a clock at some time.
func NewManualClock() *ManualClock { return &ManualClock{now: time.Unix(1e9, 0)} }

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) AfterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &manualTimer{at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		was := !t.stopped
		t.stopped = true
		return was
	}
}

// Advance moves the clock on by d and calls the functions that came due.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []func()
	live := c.timers[:0]
	for _, t := range c.timers {
		switch {
		case t.stopped:
		case !t.at.After(c.now):
			t.stopped = true
			due = append(due, t.f)
		default:
			live = append(live, t)
		}
	}
	c.timers = live
	c.mu.Unlock()
	for _, f := range due {
		f()
	}
}

// Saver decides when a window's document is saved and writes it. Its
// methods run on the window's UI goroutine (Wait excepted); the writes
// run on goroutines of their own, one at a time. A nil Saver is off.
type Saver struct {
	Clock Clock // nil: real time

	post     func(func())        // runs a function on the UI goroutine
	snapshot func() func() error // the write of the document as it is now; nil when there's nothing to save
	done     func(err error)     // a background save finished (UI goroutine)

	// UI goroutine only.
	on          bool
	pending     bool      // edits not saved (nor being saved)
	first, last time.Time // the first and last of those edits
	busy        bool      // a background save is running
	again       bool      // SaveNow while busy: save once it's done
	stop        func() bool
	gen         int    // timer generation; an older timer's call is ignored
	seq         uint64 // numbers the writes, in the order started

	mu      sync.Mutex // held while the file is written
	written uint64     // seq of the last write that succeeded
	writing sync.WaitGroup
}

// New makes the saver of a window. post runs a function on the UI
// goroutine (and wakes it); snapshot returns the write of the document's
// current state, nil when it has no unsaved changes; done is told how
// each background save ended (after an error AutoSave is already off).
func New(post func(func()), snapshot func() func() error, done func(err error)) *Saver {
	return &Saver{post: post, snapshot: snapshot, done: done}
}

func (s *Saver) clock() Clock {
	if s.Clock == nil {
		return realClock{}
	}
	return s.Clock
}

// On reports whether AutoSave is on.
func (s *Saver) On() bool { return s != nil && s.on }

// Busy reports whether a background save is running ("Saving…").
func (s *Saver) Busy() bool { return s != nil && s.busy }

// SetOn turns AutoSave on, which saves at once, or off, which forgets
// edits waiting for their save (a save already running finishes).
func (s *Saver) SetOn(on bool) {
	if s == nil || on == s.on {
		return
	}
	s.on = on
	if !on {
		s.pending, s.again = false, false
		s.stopTimer()
		return
	}
	s.SaveNow()
}

// Edited tells the saver the document changed: it is saved Delay after
// the last such call, or MaxWait after the first unsaved one.
func (s *Saver) Edited() {
	if !s.On() {
		return
	}
	now := s.clock().Now()
	if !s.pending {
		s.pending, s.first = true, now
	}
	s.last = now
	s.schedule()
}

// SaveNow saves now, or as soon as the save running ends.
func (s *Saver) SaveNow() {
	if !s.On() {
		return
	}
	s.pending = true
	if s.busy {
		s.again = true
		return
	}
	s.start()
}

// due is when the pending edits are saved.
func (s *Saver) due() time.Time {
	d := s.last.Add(Delay)
	if m := s.first.Add(MaxWait); m.Before(d) {
		d = m
	}
	return d
}

// schedule sets the timer for the pending edits. While a save runs it
// waits: finish schedules again.
func (s *Saver) schedule() {
	s.stopTimer()
	if !s.pending || s.busy {
		return
	}
	s.gen++
	gen := s.gen
	s.stop = s.clock().AfterFunc(max(s.due().Sub(s.clock().Now()), 0), func() {
		s.post(func() { s.fire(gen) })
	})
}

func (s *Saver) stopTimer() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	s.gen++
}

// fire is the timer's call, on the UI goroutine.
func (s *Saver) fire(gen int) {
	if gen != s.gen || !s.On() || !s.pending || s.busy {
		return
	}
	if s.clock().Now().Before(s.due()) {
		s.schedule() // an edit came after the timer was set
		return
	}
	s.start()
}

// start takes the snapshot and writes it in the background.
func (s *Saver) start() {
	s.stopTimer()
	s.pending, s.again = false, false
	write := s.snapshot()
	if write == nil {
		return // nothing unsaved
	}
	s.busy = true
	s.seq++
	seq := s.seq
	pending.Add(1)
	s.writing.Add(1)
	go func() {
		defer pending.Done()
		defer s.writing.Done()
		err := s.write(seq, write)
		s.post(func() { s.finish(err) })
	}()
}

// finish runs on the UI goroutine when a background save ends.
func (s *Saver) finish(err error) {
	s.busy = false
	if err != nil {
		s.on, s.pending, s.again = false, false, false
		s.stopTimer()
	}
	if s.done != nil {
		s.done(err)
	}
	switch {
	case err != nil:
	case s.again:
		s.SaveNow()
	default:
		s.schedule() // edits made while it was saving
	}
}

// write writes the file, after any write still going on; a write older
// than one already done is skipped, so the file never goes back to an
// earlier state.
func (s *Saver) write(seq uint64, write func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq < s.written {
		return nil
	}
	err := write()
	if err == nil {
		s.written = seq
	}
	return err
}

// Save is a save the user asked for (Ctrl+S), on the UI goroutine: write
// runs after a background save still going on, and takes the place of
// the edits waiting for one.
func (s *Saver) Save(write func() error) error {
	if s == nil {
		return write()
	}
	s.seq++
	err := s.write(s.seq, write)
	if err == nil {
		s.pending = false
		s.stopTimer()
	}
	return err
}

// Close turns AutoSave off for good: the window is closing.
func (s *Saver) Close() {
	if s == nil {
		return
	}
	s.SetOn(false)
}

// Wait waits for the background save running, if any, to write the
// file; its end is still to be posted to the UI goroutine (tests).
func (s *Saver) Wait() { s.writing.Wait() }

// pending counts the saves of every saver, for Flush.
var pending sync.WaitGroup

// Flush waits, at most timeout, for the saves being written, so that
// exiting doesn't cut one short.
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

// settingKey is the default's key in the settings file.
const settingKey = "autoSaveDefault"

var def struct {
	sync.Mutex
	loaded, on bool
}

// Default reports whether AutoSave starts on for the files the user opens
// or saves (that it works with); off unless the user changed it. It is
// read from the settings file once per process.
func Default() bool {
	def.Lock()
	defer def.Unlock()
	if !def.loaded {
		def.loaded = true
		def.on, _ = autorecover.Setting(settingKey).(bool)
	}
	return def.on
}

// SetDefault changes the default for every window of this process and
// saves it for later ones.
func SetDefault(on bool) error {
	Default()
	def.Lock()
	def.on = on
	def.Unlock()
	return autorecover.SaveSetting(settingKey, on)
}
