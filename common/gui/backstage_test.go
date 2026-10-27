package gui

import (
	"testing"
	"time"
)

func TestWhen(t *testing.T) {
	now := time.Date(2026, 10, 16, 14, 0, 0, 0, time.Local)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 10, 16, 9, 5, 0, 0, time.Local), "Today 9:05 AM"},
		{time.Date(2026, 10, 15, 23, 59, 0, 0, time.Local), "Yesterday 11:59 PM"},
		{time.Date(2026, 10, 12, 8, 0, 0, 0, time.Local), "Mon 8:00 AM"},
		{time.Date(2026, 3, 2, 8, 0, 0, 0, time.Local), "Mar 2"},
		{time.Date(2024, 3, 2, 8, 0, 0, 0, time.Local), "Mar 2, 2024"},
		{time.Time{}, ""},
	} {
		if got := When(c.t, now); got != c.want {
			t.Errorf("When(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}
