// Fuzz test: arbitrary Workbook streams must read without panicking.

package xlsfile

import (
	"os"
	"path/filepath"
	"testing"

	"officelite/common/ole"
)

// FuzzRead feeds mutated BIFF8 Workbook streams (seeded from the sample
// .xls files) through Read, which must return rather than panic.
func FuzzRead(f *testing.F) {
	paths, _ := filepath.Glob("../../../Samples/*/*.xls")
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if _, stream, err := ole.ReadStream(data, "Workbook"); err == nil && len(stream) < 200<<10 {
			f.Add(stream)
		}
	}
	f.Add(workbook(nil, nil))
	f.Fuzz(func(t *testing.T, stream []byte) {
		wb, err := Read(compound(t, stream))
		if err == nil && wb == nil {
			t.Fatal("nil workbook without an error")
		}
	})
}
