// zipcap_test.go checks that a zip entry larger than the size cap is an
// error (instead of being silently truncated), both for the archives Open
// needs and for data files an importer reads later.

package iwork

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
)

// capZip is a zip holding the named entries.
func capZip(t *testing.T, entries map[string][]byte) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

// TestZipEntryTooLarge lowers the entry cap to 16 bytes: an entry of exactly
// 16 bytes reads whole, one of 17 is an ErrTooLarge error — from
// readZipFile, from Open (an archive) and from Data (remembered by Err).
func TestZipEntryTooLarge(t *testing.T) {
	old := maxZipEntry
	t.Cleanup(func() { maxZipEntry = old })
	maxZipEntry = 16
	zr := capZip(t, map[string][]byte{
		"fits":               bytes.Repeat([]byte("a"), 16),
		"Data/big.png":       bytes.Repeat([]byte("b"), 17),
		"Index/Document.iwa": bytes.Repeat([]byte("c"), 17),
	})
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	if b, err := readZipFile(files["fits"]); err != nil || len(b) != 16 {
		t.Errorf("16-byte entry: got %d bytes, %v; want all 16", len(b), err)
	}
	if _, err := readZipFile(files["Data/big.png"]); !errors.Is(err, ErrTooLarge) {
		t.Errorf("17-byte entry: got %v, want ErrTooLarge", err)
	}
	if _, err := open(zr); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Open with an archive past the cap: got %v, want ErrTooLarge", err)
	}
	d := &Document{files: files, datas: map[uint64]string{7: "Data/big.png"}}
	if _, _, err := d.Data(7); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Data past the cap: got %v, want ErrTooLarge", err)
	}
	if !errors.Is(d.Err(), ErrTooLarge) {
		t.Errorf("Err after a data file past the cap: got %v, want ErrTooLarge", d.Err())
	}
}
