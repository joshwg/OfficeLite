// Command fsprobe is serverfs_test.go's WebAssembly program: under Node,
// with static/serverfs.js as its file system and a test server behind
// it, it does what WordLite and ExcelLite do to /server files (a safe
// save through common/atomicfile, reading a zip with ReadAt, stat,
// listing, renaming, deleting, lock files, folders) and checks the
// errors the server's rules turn into. It prints a line per failed check and exits
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

	folders()

	if failed > 0 {
		os.Exit(1)
	}
	fmt.Println("ok")
}

// folders checks folders: making them, a safe save and a lock file in
// one, listing, stat, moving a document and renaming a folder, and
// removing folders.
func folders() {
	check(os.MkdirAll("/server/Reports/2026", 0o755) == nil, "MkdirAll")
	fi, err := os.Stat("/server/reports/2026")
	check(err == nil && fi.IsDir(), "stat folder in another case: %v %v", fi, err)
	err = os.Mkdir("/server/Reports", 0o755)
	check(errors.Is(err, fs.ErrExist), "Mkdir existing: %v", err)
	err = os.Mkdir("/server/.hidden", 0o755)
	check(err != nil, "Mkdir of a hidden folder worked")
	err = os.Mkdir("/server/Old.docx", 0o755)
	check(err != nil, "Mkdir with a file's extension worked")
	err = os.Mkdir("/server/Missing/Sub", 0o755)
	check(errors.Is(err, fs.ErrNotExist), "Mkdir in a missing folder: %v", err)

	check(atomicfile.WriteFile("/server/Reports/2026/Plan.docx", []byte("plan"), 0o644) == nil, "safe save in a folder")
	b, err := os.ReadFile("/server/REPORTS/2026/plan.docx")
	check(err == nil && string(b) == "plan", "read back from the folder: %q %v", b, err)
	lf, err := os.OpenFile("/server/Reports/2026/~$Plan.docx", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	check(err == nil, "lock file in a folder: %v", err)
	if lf != nil {
		_ = lf.Close()
	}
	entries, err := os.ReadDir("/server/Reports/2026")
	var listed []string
	for _, e := range entries {
		listed = append(listed, e.Name())
	}
	check(err == nil && slices.Equal(listed, []string{"Plan.docx"}), "ReadDir of the folder: %v %v", listed, err)
	check(os.Remove("/server/Reports/2026/~$Plan.docx") == nil, "remove the lock file")
	entries, err = os.ReadDir("/server")
	folder := slices.IndexFunc(entries, func(e fs.DirEntry) bool { return e.Name() == "Reports" })
	check(err == nil && folder >= 0 && entries[folder].IsDir(), "ReadDir top: %v %v", entries, err)
	_, err = os.ReadDir("/server/Nowhere")
	check(errors.Is(err, fs.ErrNotExist), "ReadDir missing: %v", err)
	err = os.WriteFile("/server/Nowhere/x.docx", []byte("x"), 0o644)
	check(errors.Is(err, fs.ErrNotExist), "save into a missing folder: %v", err)

	check(os.Rename("/server/Reports/2026/Plan.docx", "/server/Reports/Plan.docx") == nil, "move a document up")
	check(os.Rename("/server/Reports/2026", "/server/Reports/Done") == nil, "rename a folder")
	fi, err = os.Stat("/server/Reports/Done")
	check(err == nil && fi.IsDir(), "renamed folder: %v %v", fi, err)
	err = os.Remove("/server/Reports")
	check(errors.Is(err, syscall.ENOTEMPTY), "remove a full folder: %v", err)
	check(os.Remove("/server/Reports/Done") == nil, "remove an empty folder")
	check(os.Remove("/server/Reports/Plan.docx") == nil, "remove the document")
	check(os.Remove("/server/Reports") == nil, "remove the emptied folder")
	_, err = os.Stat("/server/Reports")
	check(errors.Is(err, fs.ErrNotExist), "stat removed folder: %v", err)
}
