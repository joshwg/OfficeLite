package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"officelite/common/lockfile"
	"officelite/excel/model"
)

// TestSaveOverLockedFile: saving over a document Excel holds open fails
// with a message saying so, and leaves no temporary file behind.
func TestSaveOverLockedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Budget.xlsx")
	wb := &model.Workbook{Name: "Budget.xlsx", Sheets: []*model.Sheet{model.NewSheet("Sheet1")}, Styles: []model.Style{model.DefaultStyle()}}
	if err := SaveFile(wb, path, XLSX, 0); err != nil {
		t.Fatal(err)
	}
	p, _ := syscall.UTF16PtrFromString(path)
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, nil,
		syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(h)
	err = SaveFile(wb, path, XLSX, 0)
	if err == nil {
		t.Fatal("saved over a locked file")
	}
	if msg := lockfile.SaveError(path, err); !strings.Contains(msg, "Budget.xlsx is open in another program") {
		t.Errorf("message %q", msg)
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Errorf("left %v", es)
	}
}
