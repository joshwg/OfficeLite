// Package macrotest adds a (dummy) VBA project to files written by the
// OfficeLite writers, for tests of how files with macros are handled.
// Nothing outside tests imports it.
package macrotest

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"

	"officelite/common/ole"
)

// relVBA is the relationship type of an OOXML package's VBA project.
const relVBA = "http://schemas.microsoft.com/office/2006/relationships/vbaProject"

// AddToPackage returns the OOXML package data with a vbaProject.bin part
// next to the part whose relationships are in rels (such as
// "xl/_rels/workbook.xml.rels") and a relationship to it.
func AddToPackage(data []byte, rels string) ([]byte, error) {
	return AddRel(data, rels, relVBA, "vbaProject.bin", "bin")
}

// AddRel returns the OOXML package data with a part target (made up,
// relative to the part whose relationships are in rels) and a
// relationship of type typ to it.
func AddRel(data []byte, rels, typ, target, ext string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	found := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		if closeErr := rc.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
		if f.Name == rels {
			found = true
			rel := fmt.Sprintf(`<Relationship Id="rIdMacroTest" Type="%s" Target="%s"/>`, typ, target)
			b = []byte(strings.Replace(string(b), "</Relationships>", rel+"</Relationships>", 1))
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(b); err != nil {
			return nil, err
		}
	}
	if !found {
		return nil, fmt.Errorf("no %s in the package", rels)
	}
	dir := rels[:strings.LastIndex(rels, "_rels/")]
	w, err := zw.Create(dir + target)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte("not a real " + ext + " part")); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// AddToCompound returns the compound file data (.xls, .doc) with a
// _VBA_PROJECT stream added. The writer used here keeps every stream at
// the root, where Office would put it in a storage; readers only look
// for the name.
func AddToCompound(data []byte) ([]byte, error) {
	names, err := ole.Streams(data)
	if err != nil {
		return nil, err
	}
	var streams []ole.Stream
	for _, n := range names {
		_, b, err := ole.ReadStream(data, n)
		if err != nil {
			return nil, err
		}
		streams = append(streams, ole.Stream{Name: n, Data: b})
	}
	streams = append(streams, ole.Stream{Name: "_VBA_PROJECT", Data: []byte{0xCC, 0x61, 0xFF, 0xFF, 0, 0, 0}})
	var out bytes.Buffer
	if err := ole.Write(&out, streams); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
