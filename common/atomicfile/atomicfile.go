// Package atomicfile replaces files whole, so that a reader (another
// window, another OfficeLite process) never sees one half written.
package atomicfile

import (
	"bufio"
	"fmt"
	"io"
	"officelite/common/limits"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// WriteFile writes data to a temporary file in path's directory (made if
// missing) and renames it over path, which gets permissions perm. On
// Windows the rename fails while another process has path open, so it is
// retried for a moment.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return replace(path, filepath.Base(path)+".*.tmp", perm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// Write is WriteFile for content too big to hold in memory twice (a saved
// document): fill writes it, buffered, to a hidden temporary file in
// path's directory, which then replaces path. An existing path must be a
// regular file and keeps its permissions; a new one gets perm. The
// directory must exist. When fill or any step after it fails, path is
// left as it was and the temporary file is removed.
func Write(path string, perm os.FileMode, fill func(io.Writer) error) error {
	if fi, err := os.Stat(path); err == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		perm = fi.Mode().Perm()
	}
	return replace(path, "."+filepath.Base(path)+".*.tmp", perm, func(w io.Writer) error {
		bw := bufio.NewWriterSize(w, limits.AtomicWriteBuffer)
		if err := fill(bw); err != nil {
			return err
		}
		return bw.Flush()
	})
}

// replace writes a temporary file named after pattern (see os.CreateTemp)
// in path's directory with fill, syncs it, gives it permissions perm and
// renames it over path, removing it on any failure.
func replace(path, pattern string, perm os.FileMode, fill func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	tmp := f.Name()
	err = fill(f)
	if err == nil {
		// Flushed before the rename: otherwise a crash or power loss
		// soon after can leave path renamed to an empty or partial file.
		err = f.Sync()
	}
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
		if removeErr := os.Remove(tmp); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("%w (removing temporary file: %v)", err, removeErr)
		}
	}
	return err
}

// rename renames from over to, retrying briefly on Windows while another
// process holds to open.
func rename(from, to string) error {
	err := os.Rename(from, to)
	if runtime.GOOS != "windows" {
		return err
	}
	// A sharing violation (another process reading to) passes quickly.
	for i := 1; err != nil && i <= limits.RenameRetries; i++ {
		time.Sleep(time.Duration(i) * limits.RenameBackoff)
		err = os.Rename(from, to)
	}
	return err
}
