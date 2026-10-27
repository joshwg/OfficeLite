// Package fileio loads spreadsheet files (.xlsx, .xls, .csv) into a
// model.Workbook, picking the reader from the file's content.
package fileio

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"officelite/common/ole"
	"officelite/excel/fileio/csvfile"
	"officelite/excel/fileio/xlsfile"
	"officelite/excel/fileio/xlsxfile"
	"officelite/excel/model"
)

var magicZip = []byte{'P', 'K', 0x03, 0x04}

// Open reads the file at path.
func Open(path string) (*model.Workbook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Read(filepath.Base(path), data)
}

// Read parses data. The format comes from the content, not the extension:
// files named .xls are often really CSV or .xlsx in disguise.
func Read(name string, data []byte) (*model.Workbook, error) {
	var (
		wb  *model.Workbook
		err error
	)
	switch {
	case bytes.HasPrefix(data, magicZip):
		wb, err = xlsxfile.Read(data)
	case ole.IsCompoundFile(data):
		wb, err = xlsfile.Read(data)
	default:
		wb, err = csvfile.Read(name, data)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	wb.Name = name
	if len(wb.Sheets) == 0 {
		wb.Sheets = append(wb.Sheets, model.NewSheet("Sheet1"))
	}
	if len(wb.Styles) == 0 {
		wb.Styles = []model.Style{model.DefaultStyle()}
	}
	return wb, nil
}
