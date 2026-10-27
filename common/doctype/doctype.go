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

	"github.com/richardlehane/mscfb"

	"officelite/common/ole"
)

type Kind int

const (
	Unknown Kind = iota
	Spreadsheet
	Document
)

func (k Kind) String() string {
	return [...]string{"unknown", "spreadsheet", "document"}[k]
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

func detect(name string, head []byte, r io.ReaderAt, size int64) Kind {
	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		if k := detectZip(r, size); k != Unknown {
			return k
		}
	case ole.IsCompoundFile(head):
		if k := detectOLE(r); k != Unknown {
			return k
		}
	}
	return byExtension(name)
}

// detectZip looks for the main part of an Office Open XML package.
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
		}
	}
	return Unknown
}

// detectOLE looks at the stream names of a binary Office file.
func detectOLE(r io.ReaderAt) Kind {
	cf, err := mscfb.New(r)
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

func byExtension(name string) Kind {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".xlsm", ".xltx", ".xltm", ".xls", ".xlt", ".csv", ".tsv", ".tab":
		return Spreadsheet
	case ".docx", ".docm", ".dotx", ".dotm", ".doc", ".dot", ".rtf", ".txt", ".odt":
		return Document
	}
	return Unknown
}
