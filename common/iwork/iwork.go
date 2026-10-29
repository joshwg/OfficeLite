// Package iwork reads the object archives of Apple iWork documents
// (Numbers, Pages and Keynote 2013 and later). A document is a zip of
// Index/*.iwa files — Snappy-compressed streams of protobuf messages,
// each object an ArchiveInfo header (identifier and message types)
// followed by its message — plus Data/ (pictures) and Metadata/.
//
// The protobuf schemas aren't compiled in: callers read fields by number
// (see Message), following the reverse-engineered iWork .proto files
// (e.g. those of the numbers-parser and keynote-parser projects). This
// package holds what Numbers and Pages share: the archive index, data
// files, styles and text storages.
package iwork

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strings"
)

// ErrOldFormat is returned for iWork '09 and older documents (gzipped XML),
// which aren't supported.
var ErrOldFormat = errors.New("iWork '09 (or older) documents are not supported; open the file in a current Numbers or Pages and save it again, or export it to Excel or Word")

// ErrNotIWork is returned for zips that aren't iWork documents.
var ErrNotIWork = errors.New("not an iWork document")

// ErrEncrypted is returned for password-protected documents.
var ErrEncrypted = errors.New("password-protected iWork documents are not supported")

// App is the iWork application a document belongs to.
type App int

const (
	Unknown App = iota
	Numbers
	Pages
	Keynote
)

// String returns the app's name ("Numbers", "Pages", "Keynote" or "unknown").
func (a App) String() string {
	return [...]string{"unknown", "Numbers", "Pages", "Keynote"}[a]
}

// Root object types of the apps' documents (object 1 in Document.iwa).
const (
	typeAppDocument   = 1     // TN.DocumentArchive (Numbers), KN.DocumentArchive (Keynote)
	typePagesDocument = 10000 // TP.DocumentArchive
	typePackageMeta   = 11006 // TSP.PackageMetadata
)

// Object is one archived object.
type Object struct {
	ID   uint64
	Type uint32
	Msg  Message // the object's message (the first of its archive)
	File string  // the .iwa it came from
}

// Document is an opened iWork document: its objects by identifier and its
// data files.
type Document struct {
	App     App
	Objects map[uint64]*Object
	files   map[string]*zip.File
	datas   map[uint64]string // data identifier -> zip path
	styles  map[uint64]*Style
	tables  map[uint64]*Table
}

// Open reads an iWork document from the bytes of its file (a zip).
func Open(data []byte) (*Document, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	return open(zr)
}

// open indexes the objects of an opened zip (or of its inner Index.zip) and
// identifies the app, returning ErrOldFormat, ErrEncrypted or ErrNotIWork
// for packages it can't read.
func open(zr *zip.Reader) (*Document, error) {
	d := &Document{Objects: map[uint64]*Object{}, files: map[string]*zip.File{}, datas: map[uint64]string{}}
	for _, f := range zr.File {
		d.files[strings.TrimPrefix(f.Name, "/")] = f
	}
	// A package saved as a directory bundle and zipped keeps its archives
	// in an inner Index.zip.
	if f := d.files["Index.zip"]; f != nil && !d.hasIWA() {
		b, err := readZipFile(f)
		if err != nil {
			return nil, err
		}
		inner, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, err
		}
		for _, f := range inner.File {
			d.files[strings.TrimPrefix(f.Name, "/")] = f
		}
	}
	if !d.hasIWA() {
		if d.isOldFormat() {
			return nil, ErrOldFormat
		}
		if d.files[".iwph"] != nil || d.files["Index/Document.iwa.encrypted"] != nil {
			return nil, ErrEncrypted
		}
		return nil, ErrNotIWork
	}
	names := make([]string, 0, len(d.files))
	for name := range d.files {
		if strings.HasSuffix(name, ".iwa") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := readZipFile(d.files[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := d.addArchives(name, b); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	if d.Objects[1] == nil {
		if d.files[".iwph"] != nil {
			return nil, ErrEncrypted
		}
		return nil, errors.New("iwork: the document object is missing")
	}
	d.App = d.app()
	d.readDataInfo()
	return d, nil
}

// hasIWA reports whether the package has any Index/*.iwa archive.
func (d *Document) hasIWA() bool {
	for name := range d.files {
		if strings.HasPrefix(name, "Index/") && strings.HasSuffix(name, ".iwa") {
			return true
		}
	}
	return false
}

// isOldFormat recognizes iWork '08/'09 packages: a gzipped (or plain) XML
// index instead of Index/*.iwa.
func (d *Document) isOldFormat() bool {
	for _, n := range []string{"index.xml.gz", "index.xml", "index.apxl.gz", "index.apxl", "Index.xml.gz"} {
		if d.files[n] != nil {
			return true
		}
	}
	return false
}

// app identifies the document's app from the type of object 1, telling
// Keynote from Numbers by its slide archives.
func (d *Document) app() App {
	switch d.Objects[1].Type {
	case typePagesDocument:
		return Pages
	case typeAppDocument:
		for name := range d.files {
			if strings.HasPrefix(name, "Index/Slide") || strings.HasPrefix(name, "Index/MasterSlide") {
				return Keynote
			}
		}
		return Numbers
	}
	return Unknown
}

// addArchives indexes the objects of one .iwa file.
func (d *Document) addArchives(name string, raw []byte) error {
	b, err := decodeIWA(raw)
	if err != nil {
		return err
	}
	for len(b) > 0 {
		l, n := binary.Uvarint(b)
		if n <= 0 || l > uint64(len(b)-n) {
			return errors.New("iwork: bad archive header")
		}
		info := Decode(b[n : n+int(l)])
		b = b[n+int(l):]
		id := info.Uint(1)
		for i, mi := range info.Msgs(2) {
			length := mi.Uint(3)
			if length > uint64(len(b)) {
				return errors.New("iwork: truncated archive")
			}
			payload := b[:length]
			b = b[length:]
			if i == 0 {
				d.Objects[id] = &Object{ID: id, Type: uint32(mi.Uint(1)), Msg: Decode(payload), File: name}
			}
		}
	}
	return nil
}

// readDataInfo maps data identifiers (TSP.DataReference) to the files in
// Data/, from the package metadata.
func (d *Document) readDataInfo() {
	for _, o := range d.Objects {
		if o.Type != typePackageMeta {
			continue
		}
		for _, di := range o.Msg.Msgs(4) {
			id := di.Uint(1)
			for _, name := range []string{di.Str(4), di.Str(3)} {
				if name == "" {
					continue
				}
				if p := "Data/" + name; d.files[p] != nil {
					d.datas[id] = p
					break
				}
			}
		}
	}
}

// Obj returns the object with identifier id, or nil.
func (d *Document) Obj(id uint64) *Object { return d.Objects[id] }

// Msg returns the message of object id (nil if there is none).
func (d *Document) Msg(id uint64) Message {
	if o := d.Objects[id]; o != nil {
		return o.Msg
	}
	return nil
}

// Type returns the type of object id (0 if there is none).
func (d *Document) Type(id uint64) uint32 {
	if o := d.Objects[id]; o != nil {
		return o.Type
	}
	return 0
}

// OfType returns the identifiers of the objects of type t, in order.
func (d *Document) OfType(t uint32) []uint64 {
	var ids []uint64
	for id, o := range d.Objects {
		if o.Type == t {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// Data returns the contents and file name of the data file a
// TSP.DataReference identifier names.
func (d *Document) Data(id uint64) ([]byte, string, error) {
	p, ok := d.datas[id]
	if !ok {
		return nil, "", fmt.Errorf("iwork: data %d is not in the package", id)
	}
	b, err := readZipFile(d.files[p])
	return b, path.Base(p), err
}

// File returns a file of the package (e.g. "preview.jpg"), or nil.
func (d *Document) File(name string) []byte {
	f := d.files[name]
	if f == nil {
		return nil
	}
	b, _ := readZipFile(f)
	return b
}

// readZipFile returns the contents of a zip entry, capped at 1 GiB.
func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 1<<30))
}

// findDocument finds the Index/Document.iwa entry of a zip, and whether it
// has Keynote's slide archives.
func findDocument(zr *zip.Reader) (doc *zip.File, keynote bool) {
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		switch {
		case name == "Index/Document.iwa":
			doc = f
		case strings.HasPrefix(name, "Index/Slide"), strings.HasPrefix(name, "Index/MasterSlide"):
			keynote = true
		}
	}
	return doc, keynote
}

// Detect tells which iWork app a zip belongs to without decoding the
// whole document: Unknown when it isn't an iWork document. Old iWork '09
// packages are recognized by their name's extension only.
func Detect(r io.ReaderAt, size int64) App {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return Unknown
	}
	doc, keynote := findDocument(zr)
	if doc == nil {
		// A package zipped from a directory bundle keeps them in Index.zip.
		var inner *zip.Reader
		for _, f := range zr.File {
			if strings.TrimPrefix(f.Name, "/") == "Index.zip" {
				if b, err := readZipFile(f); err == nil {
					inner, _ = zip.NewReader(bytes.NewReader(b), int64(len(b)))
				}
				break
			}
		}
		if inner == nil {
			return Unknown
		}
		doc, keynote = findDocument(inner)
	}
	if doc == nil {
		return Unknown
	}
	raw, err := readZipFile(doc)
	if err != nil {
		return Unknown
	}
	d := &Document{Objects: map[uint64]*Object{}}
	_ = d.addArchives(doc.Name, raw)
	if d.Objects[1] == nil {
		return Unknown
	}
	if keynote && d.Objects[1].Type == typeAppDocument {
		return Keynote
	}
	return d.app()
}

// IsPackage reports whether a file's bytes are a zipped iWork document
// (of any version, including the unsupported iWork '09 format), going by
// the names in the zip.
func IsPackage(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	return isPackage(zr)
}

// isPackage is IsPackage for an opened zip.
func isPackage(zr *zip.Reader) bool {
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		switch {
		case strings.HasPrefix(name, "Index/") && strings.HasSuffix(name, ".iwa"),
			name == "Index.zip", name == "index.xml.gz", name == "index.xml", name == "index.apxl.gz", name == "index.apxl":
			return true
		}
	}
	return false
}
