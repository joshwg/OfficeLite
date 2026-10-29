// Tests for the window helpers: size and point formatting, menu items and
// the call queue.

package gui

import "testing"

// TestWindowHelpers checks ByteSize, OrNone, FormatPt and MenuItems.
func TestWindowHelpers(t *testing.T) {
	for n, want := range map[int64]string{0: "0 bytes", 1023: "1023 bytes", 1536: "1.5 KB", 3 << 20: "3.0 MB"} {
		if got := ByteSize(n); got != want {
			t.Errorf("ByteSize(%d) = %q, want %q", n, got, want)
		}
	}
	if OrNone("", "x") != "x" || OrNone(".", "x") != "x" || OrNone("a", "x") != "a" {
		t.Error("OrNone")
	}
	for v, want := range map[float64]string{11: "11", 10.5: "10.5", float64(float32(10.1)): "10.1", 12.35: "12.35"} {
		if got := FormatPt(v); got != want {
			t.Errorf("FormatPt(%v) = %q, want %q", v, got, want)
		}
	}
	items := MenuItems([]string{"Cut", "", "Paste"})
	if len(items) != 3 || items[0].Label != "Cut" || !items[1].Separator || items[2].Label != "Paste" {
		t.Errorf("MenuItems = %+v", items)
	}
}

// TestCallQueue checks that posted calls run in order on Run and that each
// post wakes the window.
func TestCallQueue(t *testing.T) {
	woken := 0
	q := NewCallQueue(func() { woken++ })
	var got []int
	done := make(chan bool)
	go func() {
		q.Post(func() { got = append(got, 1) })
		q.Post(func() { got = append(got, 2) })
		done <- true
	}()
	<-done
	q.Run()
	if len(got) != 2 || got[0] != 1 || got[1] != 2 || woken != 2 {
		t.Errorf("ran %v, woken %d times", got, woken)
	}
	q.Run() // nothing queued: returns at once
}

// TestCallQueuePostFromUI checks that the UI goroutine can post many
// calls without blocking, including from inside a queued call, and that
// Run runs those too.
func TestCallQueuePostFromUI(t *testing.T) {
	q := NewCallQueue(nil)
	n := 0
	for range 40 {
		q.Post(func() { n++ })
	}
	q.Post(func() {
		for range 40 {
			q.Post(func() { n++ })
		}
	})
	q.Run()
	if n != 80 {
		t.Errorf("ran %d calls, want 80", n)
	}
}
