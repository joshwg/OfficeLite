// Command fsprobe is serverfs_test.go's WebAssembly program: under Node,
// with static/serverfs.js as its file system and a test server behind
// it, it does what WordLite and ExcelLite do to /server files (a safe
// save through common/atomicfile, reading a zip with ReadAt, stat,
// listing, renaming, deleting, lock files) and checks the errors the
// server's rules turn into. It prints a line per failed check and exits
// 1 if there was any.

//go:build js

package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"syscall"

	"officelite/common/atomicfile"
)

// failed counts the failed checks.
var failed int

// check reports a failed check when ok is false.
func check(ok bool, format string, args ...any) {
	if !ok {
		failed++
		fmt.Printf("FAIL: "+format+"\n", args...)
	}
}

// main runs the checks.
func main() {
	fi, err := os.Stat("/server")
	check(err == nil && fi.IsDir(), "stat /server: %v %v", fi, err)

	// A safe save, as the apps' Save: temporary file, sync, rename.
	err = atomicfile.Write("/server/Probe.docx", 0o644, func(w io.Writer) error {
		_, err := io.WriteString(w, "hello")
		return err
	})
	check(err == nil, "atomicfile.Write: %v", err)
	b, err := os.ReadFile("/server/probe.DOCX")
	check(err == nil && string(b) == "hello", "read back in another case: %q %v", b, err)
	fi, err = os.Stat("/server/Probe.docx")
	check(err == nil && fi.Size() == 5 && fi.Mode().IsRegular(), "stat file: %v %v", fi, err)
	entries, err := os.ReadDir("/server")
	var listed []string
	for _, e := range entries {
		listed = append(listed, e.Name())
	}
	check(err == nil && slices.Equal(listed, []string{"Probe.docx"}), "ReadDir (no temporary file): %v %v", listed, err)

	// Saving again over it, and renaming.
	check(atomicfile.WriteFile("/server/Probe.docx", []byte("hello again"), 0o644) == nil, "save over")
	check(os.Rename("/server/Probe.docx", "/server/Final.docx") == nil, "rename")
	_, err = os.Stat("/server/Probe.docx")
	check(errors.Is(err, fs.ErrNotExist), "old name after rename: %v", err)

	// A zip read the way the document readers do (ReadAt, Seek).
	err = atomicfile.Write("/server/Book.xlsx", 0o644, func(w io.Writer) error {
		zw := zip.NewWriter(w)
		f, err := zw.Create("xl/workbook.xml")
		if err == nil {
			_, err = io.WriteString(f, "<workbook/>")
		}
		if err == nil {
			err = zw.Close()
		}
		return err
	})
	check(err == nil, "write zip: %v", err)
	zr, err := zip.OpenReader("/server/Book.xlsx")
	if check(err == nil, "open zip: %v", err); err == nil {
		rc, err := zr.File[0].Open()
		if err == nil {
			b, err = io.ReadAll(rc)
			_ = rc.Close()
		}
		check(err == nil && string(b) == "<workbook/>", "zip entry: %q %v", b, err)
		_ = zr.Close()
	}

	// Lock and temporary files stay in the page.
	lf, err := os.OpenFile("/server/~$Final.docx", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	check(err == nil, "create lock file: %v", err)
	_, err = os.OpenFile("/server/~$Final.docx", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	check(errors.Is(err, fs.ErrExist), "second exclusive create: %v", err)
	if lf != nil {
		_ = lf.Close()
	}
	entries, _ = os.ReadDir("/server")
	for _, e := range entries {
		check(!strings.HasPrefix(e.Name(), "~$"), "lock file reached the server: %s", e.Name())
	}
	check(os.Remove("/server/~$Final.docx") == nil, "remove lock file")

	// The server's rules, as errors.
	err = os.WriteFile("/server/Bad*.docx", []byte("x"), 0o644)
	check(errors.Is(err, syscall.EINVAL), "bad name: %v", err)
	err = os.WriteFile("/server/Big.docx", make([]byte, 2048), 0o644)
	check(errors.Is(err, syscall.ENOSPC), "over quota: %v", err)
	_, err = os.ReadFile("/etc/passwd")
	check(err != nil, "a path outside /server was read")

	// Deleting.
	check(os.Remove("/server/Final.docx") == nil, "remove")
	_, err = os.Stat("/server/Final.docx")
	check(errors.Is(err, fs.ErrNotExist), "stat after remove: %v", err)

	if failed > 0 {
		os.Exit(1)
	}
	fmt.Println("ok")
}
