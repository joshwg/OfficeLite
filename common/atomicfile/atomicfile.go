// Package atomicfile replaces files whole, so that a reader (another
// window, another OfficeLite process) never sees one half written.
package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// WriteFile writes data to a temporary file in path's directory (made if
// missing) and renames it over path. On Windows the rename fails while
// another process has path open, so it is retried for a moment.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, perm)
	}
	if err == nil {
		err = rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

func rename(from, to string) error {
	err := os.Rename(from, to)
	if runtime.GOOS != "windows" {
		return err
	}
	// A sharing violation (another process reading to) passes quickly.
	for i := 1; err != nil && i <= 8; i++ {
		time.Sleep(time.Duration(i) * 10 * time.Millisecond)
		err = os.Rename(from, to)
	}
	return err
}
