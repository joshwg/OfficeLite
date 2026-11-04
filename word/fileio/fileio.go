// Package fileio opens and saves Word documents, picking the format from
// the file's content (.docx zip, .doc compound file, or plain text); Pages
// documents (.pages) are opened read-only. Templates (.dotx, .dot) read
// like the documents they're built on and save with the template marker.
package fileio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"officelite/common/atomicfile"
	"officelite/common/iwork"
	"officelite/common/ole"
	"officelite/word/fileio/docfile"
	"officelite/word/fileio/docxfile"
	"officelite/word/fileio/pagesfile"
	"officelite/word/model"
)

// Format is a file format a document can be saved in.
type Format int

const (
	DOCX Format = iota // Word Document
	DOC                // Word 97-2003 Document
	TXT                // plain text (UTF-8)
	DOTX               // Word Template
	DOT                // Word 97-2003 Template
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
	case ".dotx", ".dotm":
		return DOTX, true
	case ".dot":
		return DOT, true
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
	case DOTX:
		return ".dotx"
	case DOT:
		return ".dot"
	}
	return ".docx"
}

// IsTemplate reports whether f saves a template.
func (f Format) IsTemplate() bool { return f == DOTX || f == DOT }

// Document is the document format a template format is built on: .docx
// for .dotx and .doc for .dot; other formats are returned unchanged.
func (f Format) Document() Format {
	switch f {
	case DOTX:
		return DOCX
	case DOT:
		return DOC
	}
	return f
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
	case DOTX:
		return "Word Template (*.dotx)"
	case DOT:
		return "Word 97-2003 Template (*.dot)"
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
func Read(name string, data []byte) (doc *model.Document, err error) {
	// A reader bug on a damaged file must not take the app down with it.
	defer func() {
		if r := recover(); r != nil {
			doc, err = nil, fmt.Errorf("the file is damaged or not supported (internal error: %v)", r)
		}
	}()
	switch {
	case len(data) >= 4 && string(data[:4]) == "PK\x03\x04" && iwork.IsPackage(data):
		doc, err = pagesfile.Read(data) // read-only: FormatFor has no .pages
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
	case DOTX:
		return docxfile.WriteTemplate(doc, w)
	case DOT:
		return docfile.WriteTemplate(doc, w)
	}
	return fmt.Errorf("unknown format %v", f)
}

// SaveFile writes doc to path through a temporary file renamed over the
// target (see atomicfile.Write), so a failed save leaves the old file
// intact; an existing file keeps its permissions.
func SaveFile(doc *model.Document, path string, f Format) error {
	return atomicfile.Write(path, 0o644, func(w io.Writer) error { return Write(doc, w, f) })
}
