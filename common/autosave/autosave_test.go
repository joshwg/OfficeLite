package autosave

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"officelite/common/autorecover"
)

// rig is a window: a document whose rev counts edits, saved by a Saver.
type rig struct {
	t      *testing.T
	clock  *ManualClock
	s      *Saver
	mu     sync.Mutex
	calls  []func()
	rev    int
	saved  []int // revs written, in order
	errs   []error
	fail   error         // the next writes fail with it
	block  chan struct{} // writes wait for it when set
	pendOK bool          // unsaved changes
}

func newRig(t *testing.T) *rig {
	r := &rig{t: t, clock: NewManualClock()}
	r.s = New(r.post, r.snapshot, func(err error) { r.errs = append(r.errs, err) })
	r.s.Clock = r.clock
	return r
}

func (r *rig) post(f func()) {
	r.mu.Lock()
	r.calls = append(r.calls, f)
	r.mu.Unlock()
}

// run runs what was posted, as frames do, until the saves started are
// written and their ends run.
func (r *rig) run() {
	for {
		r.s.Wait()
		r.mu.Lock()
		calls := r.calls
		r.calls = nil
		r.mu.Unlock()
		if len(calls) == 0 {
			return
		}
		for _, f := range calls {
			f()
		}
	}
}

func (r *rig) snapshot() func() error {
	if !r.pendOK {
		return nil
	}
	rev, fail, block := r.rev, r.fail, r.block
	return func() error {
		if block != nil {
			<-block
		}
		if fail != nil {
			return fail
		}
		r.mu.Lock()
		r.saved = append(r.saved, rev)
		r.mu.Unlock()
		return nil
	}
}

func (r *rig) edit() {
	r.rev++
	r.pendOK = true
	r.s.Edited()
}

// tick advances the clock and runs the posted calls.
func (r *rig) tick(d time.Duration) {
	r.clock.Advance(d)
	r.run()
}

func (r *rig) wantSaved(what string, revs ...int) {
	r.t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !slices.Equal(r.saved, revs) {
		r.t.Errorf("%s: saved %v, want %v", what, r.saved, revs)
	}
}

// TestDebounce: a save comes Delay after the last edit, each edit
// putting it off.
func TestDebounce(t *testing.T) {
	r := newRig(t)
	r.edit() // off: nothing
	r.tick(10 * time.Second)
	r.wantSaved("off")

	r.s.SetOn(true) // saves at once
	r.run()
	r.wantSaved("turned on", 1)
	r.pendOK = false

	r.edit()
	r.tick(2 * time.Second)
	r.edit() // puts it off
	r.tick(2 * time.Second)
	r.wantSaved("within the delay", 1)
	r.tick(time.Second)
	r.wantSaved("after the delay", 1, 3)
	if len(r.errs) != 2 || r.errs[1] != nil {
		t.Errorf("done %v", r.errs)
	}
	r.tick(time.Minute)
	r.wantSaved("no edits", 1, 3)
}

// TestPeriodic: edits that never pause for Delay are still saved every
// MaxWait.
func TestPeriodic(t *testing.T) {
	r := newRig(t)
	r.s.SetOn(true) // nothing unsaved: no write
	r.run()
	for range 70 {
		r.edit()
		r.tick(time.Second)
	}
	r.wantSaved("continuous editing", 60)
	r.tick(Delay)
	r.wantSaved("after the last edit", 60, 70)
}

// TestEditWhileSaving: an edit during a save is saved after it; Save
// (Ctrl+S) waits for the save running, and an older write never lands
// over a newer one.
func TestEditWhileSaving(t *testing.T) {
	r := newRig(t)
	r.s.SetOn(true)
	b := make(chan struct{})
	r.block = b
	r.edit()
	r.clock.Advance(Delay)
	waitPosted(r) // starts the save, which blocks
	if !r.s.Busy() {
		t.Fatal("not saving")
	}
	r.block = nil
	r.edit() // rev 2, during the save: no timer yet
	r.clock.Advance(Delay)
	close(b)
	r.run() // the first save ends and the second is scheduled
	r.wantSaved("first", 1)
	r.tick(0)
	r.wantSaved("second", 1, 2)

	b = make(chan struct{})
	r.block = b
	r.edit() // rev 3
	r.clock.Advance(Delay)
	waitPosted(r)
	r.block = nil
	for r.s.mu.TryLock() { // until the save holds the file
		r.s.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	go func() { time.Sleep(20 * time.Millisecond); close(b) }()
	if err := r.s.Save(func() error {
		r.mu.Lock()
		r.saved = append(r.saved, 30)
		r.mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r.run()
	r.wantSaved("Ctrl+S after the save running", 1, 2, 3, 30)

	r.s.written = r.s.seq + 2 // as if a later write had got in first
	r.edit()
	r.tick(Delay)
	r.wantSaved("older write skipped", 1, 2, 3, 30)
}

// TestError: a failed save turns AutoSave off and is reported; later
// edits aren't saved.
func TestError(t *testing.T) {
	r := newRig(t)
	r.s.SetOn(true)
	r.fail = errors.New("locked")
	r.edit()
	r.tick(Delay)
	if r.s.On() || len(r.errs) != 1 || r.errs[0] == nil {
		t.Fatalf("on %v, done %v", r.s.On(), r.errs)
	}
	r.fail = nil
	r.edit()
	r.tick(time.Minute)
	r.wantSaved("after the error")
	r.s.SetOn(true) // on again: saves what's unsaved
	r.run()
	r.wantSaved("on again", 2)
}

// waitPosted runs the timer's posted call, which starts a save that may
// block.
func waitPosted(r *rig) {
	for {
		r.mu.Lock()
		calls := r.calls
		r.calls = nil
		r.mu.Unlock()
		if len(calls) > 0 {
			for _, f := range calls {
				f()
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	old := autorecover.SettingsPath
	autorecover.SettingsPath = func() string { return path }
	reset := func() { def.loaded = false }
	t.Cleanup(func() { autorecover.SettingsPath = old; reset() })
	reset()
	if Default() {
		t.Error("on by default")
	}
	os.WriteFile(path, []byte(`{"autoRecoverMinutes": 2}`), 0o644)
	if err := SetDefault(true); err != nil {
		t.Fatal(err)
	}
	reset()
	var s map[string]any
	b, _ := os.ReadFile(path)
	json.Unmarshal(b, &s)
	if !Default() || s["autoRecoverMinutes"] != 2.0 {
		t.Errorf("saved %s", b)
	}
}
