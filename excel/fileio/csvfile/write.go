package csvfile

import (
	"bufio"
	"io"
	"strings"
	"unicode/utf8"

	"officelite/excel/model"
)

// utf8BOM starts every file Write produces. Excel's "CSV UTF-8" format
// writes it too, and without it Excel reads the file as the ANSI code page
// and garbles anything non-ASCII.
const utf8BOM = "\xEF\xBB\xBF"

// Write saves sh the way Excel's "Save as CSV" does: each cell's displayed
// text (number formats applied), every row padded with empty fields to the
// sheet's used width, CRLF line endings, and fields quoted only when they
// contain the delimiter, a quote, CR or LF. Output starts at A1 (Excel
// starts at the used range's top-left, which differs only for sheets whose
// first rows or columns are empty). Formulas are lost: only values are saved.
func Write(sh *model.Sheet, w io.Writer, delim rune) error {
	bw := bufio.NewWriterSize(w, 64<<10)
	bw.WriteString(utf8BOM)
	var d [utf8.UTFMax]byte
	sep := string(d[:utf8.EncodeRune(d[:], delim)])
	special := sep + "\"\r\n"

	rows, cols := sh.UsedRows(), sh.UsedCols()
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if c > 0 {
				bw.WriteString(sep)
			}
			text := sh.Get(r, c).Text
			if !strings.ContainsAny(text, special) {
				bw.WriteString(text)
				continue
			}
			bw.WriteByte('"')
			bw.WriteString(strings.ReplaceAll(text, `"`, `""`))
			bw.WriteByte('"')
		}
		if _, err := bw.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return bw.Flush()
}
