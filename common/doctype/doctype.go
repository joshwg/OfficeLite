// Package doctype tells spreadsheets from word-processing documents, so a
// file can be routed to the right app.
package doctype

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"officelite/common/iwork"
	"officelite/common/ole"
)

// Kind is what a file holds, and so which app opens it.
type Kind int

const (
	Unknown Kind = iota
	Spreadsheet
	Document
)

// String is the kind's lower-case name.
func (k Kind) String() string {
	names := [...]string{"unknown", "spreadsheet", "document"}
	if k < 0 || int(k) >= len(names) {
		return names[Unknown]
	}
	return names[k]
}

// DetectFile inspects the file at path, reading only what it needs: the
// zip or compound-file directory, not the whole file.
func DetectFile(path string) (Kind, error) {
	f, err := os.Open(path)
	if err != nil {
		return Unknown, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Unknown, err
	}
	head := make([]byte, 8)
	n, _ := io.ReadFull(f, head)
	return detect(filepath.Base(path), head[:n], f, st.Size()), nil
}

// Detect inspects a file already in memory.
func Detect(name string, data []byte) Kind {
	return detect(name, data, bytes.NewReader(data), int64(len(data)))
}

// detect tells the kind from the file's content (head is its first bytes),
// falling back to name's extension.
func detect(name string, head []byte, r io.ReaderAt, size int64) (k Kind) {
	// A damaged file falls back to its extension rather than crashing;
	// opening it then reports the damage.
	defer func() {
		if recover() != nil {
			k = byExtension(name)
		}
	}()
	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		if k := detectZip(r, size); k != Unknown {
			return k
		}
	case ole.IsCompoundFile(head):
		if k := detectOLE(r, size); k != Unknown {
			return k
		}
	}
	return byExtension(name)
}

// detectZip looks for the main part of an Office Open XML package, or
// the document archive of a Numbers or Pages file.
func detectZip(r io.ReaderAt, size int64) Kind {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return Unknown
	}
	for _, f := range zr.File {
		switch strings.ToLower(strings.TrimPrefix(f.Name, "/")) {
		case "xl/workbook.xml", "xl/workbook.bin":
			return Spreadsheet
		case "word/document.xml":
			return Document
		case "index/document.iwa":
			switch iwork.Detect(r, size) {
			case iwork.Numbers:
				return Spreadsheet
			case iwork.Pages:
				return Document
			}
		}
	}
	return Unknown
}

// detectOLE looks at the stream names of a binary Office file.
func detectOLE(r io.ReaderAt, size int64) Kind {
	cf, err := ole.Open(r, size)
	if err != nil {
		return Unknown
	}
	for e, err := cf.Next(); err == nil; e, err = cf.Next() {
		switch strings.ToLower(e.Name) {
		case "workbook", "book":
			return Spreadsheet
		case "worddocument":
			return Document
		}
	}
	return Unknown
}

// byExtension tells the kind from name's extension alone.
func byExtension(name string) Kind {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".xlsm", ".xltx", ".xltm", ".xls", ".xlt", ".csv", ".tsv", ".tab", ".numbers":
		return Spreadsheet
	case ".docx", ".docm", ".dotx", ".dotm", ".doc", ".dot", ".rtf", ".txt", ".odt", ".pages":
		return Document
	}
	return Unknown
}
