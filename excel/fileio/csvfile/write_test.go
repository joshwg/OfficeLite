package csvfile

import (
	"bytes"
	"testing"

	"officelite/excel/model"
)

func TestWrite(t *testing.T) {
	sh := model.NewSheet("S")
	text := func(r, c int, s string) { sh.Set(r, c, model.Cell{Text: s, Kind: model.KindText}) }
	text(0, 0, "plain")
	text(0, 1, "a,b")
	text(0, 2, `say "hi"`)
	sh.Set(1, 0, model.Cell{Text: "1,234.50", Num: 1234.5, Kind: model.KindNumber})
	text(1, 1, "two\nlines")
	text(1, 3, " spaced ")
	sh.Set(2, 4, model.Cell{Style: 1}) // a formatted empty cell widens the used range
	text(3, 0, "é\tx")

	var buf bytes.Buffer
	if err := Write(sh, &buf, ','); err != nil {
		t.Fatal(err)
	}
	want := "\xEF\xBB\xBF" +
		`plain,"a,b","say ""hi""",,` + "\r\n" +
		`"1,234.50","two` + "\n" + `lines",, spaced ,` + "\r\n" +
		",,,,\r\n" +
		"é\tx,,,,\r\n"
	if buf.String() != want {
		t.Errorf("csv =\n%q\nwant\n%q", buf.String(), want)
	}

	buf.Reset()
	if err := Write(sh, &buf, '\t'); err != nil {
		t.Fatal(err)
	}
	if want := "\"é\tx\"\t\t\t\t\r\n"; !bytes.HasSuffix(buf.Bytes(), []byte(want)) {
		t.Errorf("tsv ends %q, want %q", buf.String(), want)
	}

	// What was written reads back as the same text.
	wb, err := Read("x.csv", bytesOf(t, sh, ','))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range [][2]int{{0, 1}, {0, 2}, {1, 1}, {1, 3}, {3, 0}} {
		if got, want := wb.Sheets[0].Get(ref[0], ref[1]).Text, sh.Get(ref[0], ref[1]).Text; got != want {
			t.Errorf("%s read back %q, want %q", model.CellName(ref[0], ref[1]), got, want)
		}
	}
}

func bytesOf(t *testing.T, sh *model.Sheet, delim rune) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(sh, &buf, delim); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWriteEmpty(t *testing.T) {
	if got := bytesOf(t, model.NewSheet("S"), ','); string(got) != "\xEF\xBB\xBF" {
		t.Errorf("empty sheet = %q", got)
	}
}
