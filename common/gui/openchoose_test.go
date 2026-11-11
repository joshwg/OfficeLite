// Tests for ChooseOpen's routing of the fallback dialog's outcomes.

package gui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestChooseOpen checks a canceled dialog calls nothing, a failing one
// calls Fail, a file with a path goes to Open (closed) and one without
// goes to Read.
func TestChooseOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.docx")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		choose func(...string) (io.ReadCloser, error)
		want   string
	}{
		{"canceled", func(...string) (io.ReadCloser, error) { return nil, errDeclined }, ""},
		{"failed", func(...string) (io.ReadCloser, error) { return nil, errors.New("boom") }, "fail:boom"},
		{"path", func(...string) (io.ReadCloser, error) { return os.Open(path) }, "open:" + path},
		{"data", func(...string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("d")), nil }, "read"},
	} {
		q := NewCallQueue(nil)
		got := make(chan string, 1)
		ChooseOpen{
			Choose:   c.choose,
			Declined: errDeclined,
			Calls:    q,
			Open:     func(p string) { got <- "open:" + p },
			Read: func(rc io.ReadCloser) {
				CloseChosen(rc)
				got <- "read"
			},
			Fail: func(err error) { got <- "fail:" + err.Error() },
		}.Start()
		var res string
		if c.want == "" {
			time.Sleep(50 * time.Millisecond) // nothing should come
			q.Run()
			select {
			case res = <-got:
			default:
			}
		} else {
			runUntil(t, q, func() bool {
				select {
				case res = <-got:
					return true
				default:
					return false
				}
			})
		}
		if res != c.want {
			t.Errorf("%s: %q, want %q", c.name, res, c.want)
		}
	}
}
