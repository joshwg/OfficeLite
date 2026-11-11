// partcap.go caps how much of each zip entry (part) the .xlsx reader and
// writer read themselves: excel.XlsxMaxPartSize. A part over the cap is an
// error ("file is damaged or too large"), never a silently cut part; at
// most cap+1 bytes are read to tell.

package xlsxfile

import (
	"archive/zip"
	"fmt"
	"io"
)

// partTooLargeError reports a part whose uncompressed size is over the cap.
type partTooLargeError struct {
	part  string // the zip entry's name
	limit int64  // the cap it went over
}

// Error returns the message the user sees.
func (e *partTooLargeError) Error() string {
	return fmt.Sprintf("file is damaged or too large: part %s is larger than %s", e.part, sizeText(e.limit))
}

// sizeText writes n bytes in the largest whole unit: 1 GB, 64 MB, 3 KB or
// 100 bytes.
func sizeText(n int64) string {
	switch {
	case n >= 1<<30 && n%(1<<30) == 0:
		return fmt.Sprintf("%d GB", n>>30)
	case n >= 1<<20 && n%(1<<20) == 0:
		return fmt.Sprintf("%d MB", n>>20)
	case n >= 1<<10 && n%(1<<10) == 0:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

// cappedReader reads a zip entry and fails with a *partTooLargeError once
// it has yielded more than limit bytes; the bytes past the cap are never
// handed out.
type cappedReader struct {
	r        io.Reader // the entry, limited to limit+1 bytes
	c        io.Closer // the entry itself
	part     string
	limit, n int64
	onOver   func(*partTooLargeError) // told once when the cap is passed (may be nil)
	err      *partTooLargeError       // set once the cap is passed
}

// openPart opens zip entry f, capped at limit bytes; onOver (if not nil)
// is told when the entry turns out larger.
func openPart(f *zip.File, limit int64, onOver func(*partTooLargeError)) (io.ReadCloser, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	return &cappedReader{r: io.LimitReader(rc, limit+1), c: rc, part: f.Name, limit: limit, onOver: onOver}, nil
}

// Read reads from the entry, failing once more than limit bytes came.
func (c *cappedReader) Read(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > c.limit {
		n -= int(c.n - c.limit)
		c.err = &partTooLargeError{part: c.part, limit: c.limit}
		if c.onOver != nil {
			c.onOver(c.err)
		}
		return n, c.err
	}
	return n, err
}

// Close closes the entry.
func (c *cappedReader) Close() error { return c.c.Close() }
