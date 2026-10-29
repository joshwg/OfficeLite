package gui

import "testing"

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
