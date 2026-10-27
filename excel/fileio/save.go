package fileio

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"officelite/excel/fileio/csvfile"
	"officelite/excel/fileio/xlsfile"
	"officelite/excel/fileio/xlsxfile"
	"officelite/excel/model"
)

// Format is a file format a workbook can be saved in.
type Format int

const (
	XLSX Format = iota // Excel workbook, every sheet
	CSV                // comma separated values, one sheet
	TSV                // tab-delimited text, one sheet
	XLS                // Excel 97-2003 workbook (BIFF8), every sheet
)

// FormatFor picks the save format from a file name's extension. It reports
// false for formats that cannot be written.
func FormatFor(name string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".xlsm":
		return XLSX, true
	case ".csv":
		return CSV, true
	case ".tsv", ".txt":
		return TSV, true
	case ".xls":
		return XLS, true
	}
	return 0, false
}

// Ext is the format's usual file extension, dot included.
func (f Format) Ext() string {
	switch f {
	case CSV:
		return ".csv"
	case TSV:
		return ".txt"
	case XLS:
		return ".xls"
	}
	return ".xlsx"
}

// String is the format's name as Excel's Save As dialog shows it.
func (f Format) String() string {
	switch f {
	case XLSX:
		return "Excel Workbook (*.xlsx)"
	case CSV:
		return "CSV (Comma delimited) (*.csv)"
	case TSV:
		return "Text (Tab delimited) (*.txt)"
	case XLS:
		return "Excel 97-2003 Workbook (*.xls)"
	}
	return fmt.Sprintf("Format(%d)", int(f))
}

// MultiSheet reports whether the format holds the whole workbook rather
// than one sheet.
func (f Format) MultiSheet() bool { return f == XLSX || f == XLS }

// Write encodes wb in format f. sheet is the index of the sheet written by
// the single-sheet formats (CSV, TSV) and is ignored for XLSX.
func Write(wb *model.Workbook, w io.Writer, f Format, sheet int) error {
	switch f {
	case XLSX:
		return xlsxfile.Write(wb, w)
	case XLS:
		return xlsfile.Write(wb, w)
	case CSV, TSV:
		if sheet < 0 || sheet >= len(wb.Sheets) {
			return fmt.Errorf("sheet %d out of range (workbook has %d)", sheet, len(wb.Sheets))
		}
		delim := ','
		if f == TSV {
			delim = '\t'
		}
		return csvfile.Write(wb.Sheets[sheet], w, delim)
	}
	return fmt.Errorf("unknown format %d", int(f))
}

// SaveFile writes wb to path. The data goes to a temporary file in the same
// directory, which then replaces path, so a failed save never leaves a
// half-written file behind. An existing file's permissions are kept.
func SaveFile(wb *model.Workbook, path string, f Format, sheet int) (err error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	bw := bufio.NewWriterSize(tmp, 256<<10)
	if err = Write(wb, bw, f, sheet); err != nil {
		return err
	}
	if err = bw.Flush(); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Fits lists what of wb format f cannot hold (rows, columns, styles,
// formulas ...); it is empty when nothing is lost beyond what any save in
// that format loses.
func Fits(wb *model.Workbook, f Format) []string {
	if f == XLS {
		return xlsfile.Fits(wb)
	}
	return nil
}
