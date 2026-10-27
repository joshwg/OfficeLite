// Package fileio opens and saves Word documents, picking the format from
// the file's content (.docx zip, .doc compound file, or plain text).
package fileio

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"officelite/common/ole"
	"officelite/word/fileio/docfile"
	"officelite/word/fileio/docxfile"
	"officelite/word/model"
)

// Format is a file format a document can be saved in.
type Format int

const (
	DOCX Format = iota // Word Document
	DOC                // Word 97-2003 Document
	TXT                // plain text (UTF-8)
)

// FormatFor picks the save format from a file name's extension.
func FormatFor(name string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".docx", ".docm":
		return DOCX, true
	case ".doc":
		return DOC, true
	case ".txt":
		return TXT, true
	}
	return 0, false
}

// Ext is the format's usual extension, dot included.
func (f Format) Ext() string {
	switch f {
	case DOC:
		return ".doc"
	case TXT:
		return ".txt"
	}
	return ".docx"
}

// String is the format's name as Word's Save As dialog shows it.
func (f Format) String() string {
	switch f {
	case DOCX:
		return "Word Document (*.docx)"
	case DOC:
		return "Word 97-2003 Document (*.doc)"
	case TXT:
		return "Plain Text (*.txt)"
	}
	return fmt.Sprintf("Format(%d)", int(f))
}

// Open reads the document at path.
func Open(path string) (*model.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Read(filepath.Base(path), data)
}

// Read decodes a document from its bytes; name is used for the document
// name and, when the content is ambiguous, its extension.
func Read(name string, data []byte) (*model.Document, error) {
	var doc *model.Document
	var err error
	switch {
	case len(data) >= 4 && string(data[:4]) == "PK\x03\x04":
		doc, err = docxfile.Read(data)
	case ole.IsCompoundFile(data):
		doc, err = docfile.Read(data)
	default:
		doc, err = readText(data), nil
	}
	if err != nil {
		return nil, err
	}
	doc.Name = name
	return doc, nil
}

// Write encodes doc in format f.
func Write(doc *model.Document, w io.Writer, f Format) error {
	switch f {
	case DOCX:
		return docxfile.Write(doc, w)
	case DOC:
		return docfile.Write(doc, w)
	case TXT:
		return writeText(doc, w)
	}
	return fmt.Errorf("unknown format %v", f)
}

// SaveFile writes doc to path through a temporary file renamed over the
// target, so a failed save leaves the old file intact.
func SaveFile(doc *model.Document, path string, f Format) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	bw := bufio.NewWriter(tmp)
	if err := Write(doc, bw, f); err != nil {
		tmp.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if fi, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmp.Name(), fi.Mode().Perm())
	} else {
		_ = os.Chmod(tmp.Name(), 0o644)
	}
	return os.Rename(tmp.Name(), path)
}
