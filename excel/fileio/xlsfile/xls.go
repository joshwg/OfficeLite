package xlsfile

import (
	"errors"
	"fmt"
	"strings"

	"officelite/common/ole"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

type font struct {
	name      string
	heightTw  int // twips (1/20 pt)
	italic    bool
	strike    bool
	bold      bool
	underline bool
	color     int // palette index
}

type xf struct {
	font, format         int
	hAlign, vAlign, wrap int
	rotation, indent     int
	border               [4]int // styles, model.Left.. order
	borderColor          [4]int
	pattern              int
	fore, back           int
}

type boundSheet struct {
	name   string
	pos    int
	hidden bool
	kind   int // 0 = worksheet
}

type parser struct {
	stream   []byte
	wb       *model.Workbook
	date1904 bool
	fonts    []font
	formats  map[int]string
	xfs      []xf
	palette  map[int]uint32 // overrides from the PALETTE record
	sheets   []boundSheet
	sst      []string
	xfStyle  []uint32 // xf index -> model style index
	nf       *numfmt.Formatter

	// Formula context: supporting books, EXTERNSHEET and NAME records.
	supbooks []supbook
	xti      []xti
	names    []definedName
	warned   map[string]bool
}

// warnf adds a workbook warning once.
func (p *parser) warnf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if p.warned == nil {
		p.warned = map[string]bool{}
	}
	if !p.warned[s] {
		p.warned[s] = true
		p.wb.Warnings = append(p.wb.Warnings, s)
	}
}

// Read parses a BIFF8 workbook held in an OLE2 compound file.
func Read(data []byte) (*model.Workbook, error) {
	name, stream, err := ole.ReadStream(data, "Workbook", "Book")
	if err != nil {
		if names, _ := ole.Streams(data); containsFold(names, "EncryptionInfo") {
			return nil, errors.New("the workbook is password-protected")
		}
		if names, _ := ole.Streams(data); containsFold(names, "WordDocument") {
			return nil, errors.New("this is a Word document, not a spreadsheet")
		}
		return nil, err
	}
	if name == "Book" {
		return nil, errors.New("Excel 5.0/95 workbooks (BIFF5) are not supported; save as .xls 97-2003 or .xlsx")
	}
	p := &parser{stream: stream, wb: &model.Workbook{}, formats: map[int]string{}, palette: map[int]uint32{}}
	if err := p.globals(); err != nil {
		return nil, err
	}
	p.buildStyles()
	p.definedNames()
	p.wb.Date1904 = p.date1904
	p.nf = numfmt.New(p.date1904)
	defer p.nf.Close()
	for _, bs := range p.sheets {
		if bs.kind != 0 {
			continue // chart, macro and VB module sheets
		}
		sh := model.NewSheet(bs.name)
		sh.Hidden = bs.hidden
		p.readSheet(bs.pos, sh)
		p.wb.Sheets = append(p.wb.Sheets, sh)
	}
	return p.wb, nil
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// globals reads the workbook globals substream: fonts, formats, XFs,
// palette, sheet directory and shared strings.
func (p *parser) globals() error {
	var (
		err     error
		first   = true
		sstSegs [][]byte
		inSST   bool
	)
	records(p.stream, 0, func(r record) bool {
		if first {
			first = false
			if r.id != recBOF || u16(r.body, 0) != biff8Version {
				err = fmt.Errorf("unsupported .xls version (only Excel 97-2003 BIFF8 is supported)")
				return false
			}
			return true
		}
		if r.id == recContinue && inSST {
			sstSegs = append(sstSegs, r.body)
			return true
		}
		inSST = false
		switch r.id {
		case recFilePass:
			err = errors.New("the workbook is password-protected")
			return false
		case recDateMode:
			p.date1904 = u16(r.body, 0) == 1
		case recFont:
			p.fonts = append(p.fonts, parseFont(r.body))
		case recFormat:
			code, _ := unicodeString(r.body, 2, 2)
			p.formats[u16(r.body, 0)] = code
		case recXF:
			p.xfs = append(p.xfs, parseXF(r.body))
		case recPalette:
			n := u16(r.body, 0)
			for i := 0; i < n; i++ {
				b := r.body[2+4*i:]
				if len(b) < 3 {
					break
				}
				p.palette[8+i] = uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
			}
		case recBoundSheet:
			name, _ := unicodeString(r.body, 6, 1)
			p.sheets = append(p.sheets, boundSheet{
				name:   name,
				pos:    int(u32(r.body, 0)),
				hidden: r.body[4]&0x03 != 0,
				kind:   int(r.body[5]),
			})
		case recSST:
			sstSegs = [][]byte{r.body[min(8, len(r.body)):]}
			inSST = true
		case recSupBook:
			p.supbooks = append(p.supbooks, parseSupBook(r.body))
			if sb := &p.supbooks[len(p.supbooks)-1]; sb.kind == supExternal {
				for _, b := range p.supbooks {
					if b.kind == supExternal {
						sb.ext++
					}
				}
			}
		case recExternName:
			if n := len(p.supbooks); n > 0 {
				name, _ := shortString(r.body, 6)
				p.supbooks[n-1].names = append(p.supbooks[n-1].names, name)
			}
		case recExternSheet:
			for i := range u16(r.body, 0) {
				o := 2 + 6*i
				if o+6 > len(r.body) {
					break
				}
				p.xti = append(p.xti, xti{u16(r.body, o), int(int16(u16(r.body, o+2))), int(int16(u16(r.body, o+4)))})
			}
		case recName:
			p.names = append(p.names, parseName(r.body))
		case recEOF:
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	if sstSegs != nil {
		s := &segments{segs: sstSegs}
		for !s.done() {
			p.sst = append(p.sst, s.str())
		}
	}
	return nil
}

func parseFont(b []byte) font {
	grbit := u16(b, 2)
	f := font{
		heightTw:  u16(b, 0),
		italic:    grbit&0x02 != 0,
		strike:    grbit&0x08 != 0,
		color:     u16(b, 4),
		bold:      u16(b, 6) >= 700,
		underline: len(b) > 10 && b[10] != 0,
	}
	f.name, _ = unicodeString(b, 14, 1)
	return f
}

func parseXF(b []byte) xf {
	if len(b) < 20 {
		return xf{}
	}
	align := int(b[6])
	b1, b2 := u32(b, 10), u32(b, 14)
	colors := u16(b, 18)
	return xf{
		font:     u16(b, 0),
		format:   u16(b, 2),
		hAlign:   align & 0x07,
		wrap:     (align >> 3) & 1,
		vAlign:   (align >> 4) & 0x07,
		rotation: int(b[7]),
		indent:   int(b[8] & 0x0F),
		border: [4]int{
			model.Left:   int(b1 & 0xF),
			model.Right:  int(b1 >> 4 & 0xF),
			model.Top:    int(b1 >> 8 & 0xF),
			model.Bottom: int(b1 >> 12 & 0xF),
		},
		borderColor: [4]int{
			model.Left:   int(b1 >> 16 & 0x7F),
			model.Right:  int(b1 >> 23 & 0x7F),
			model.Top:    int(b2 & 0x7F),
			model.Bottom: int(b2 >> 7 & 0x7F),
		},
		pattern: int(b2 >> 26 & 0x3F),
		fore:    colors & 0x7F,
		back:    colors >> 7 & 0x7F,
	}
}

func parseSupBook(b []byte) supbook {
	ctab, cch := u16(b, 0), u16(b, 2)
	switch cch {
	case 0x0401:
		return supbook{kind: supSelf}
	case 0x3A01:
		return supbook{kind: supAddin}
	}
	sb := supbook{kind: supExternal}
	off := 2
	sb.path, off = unicodeString(b, off, 2)
	for range ctab {
		if off >= len(b) {
			break
		}
		var s string
		s, off = unicodeString(b, off, 2)
		sb.sheets = append(sb.sheets, s)
	}
	return sb
}

func parseName(b []byte) definedName {
	if len(b) < 15 {
		return definedName{}
	}
	n := definedName{flags: u16(b, 0), itab: u16(b, 8)}
	cch, cce := int(b[3]), u16(b, 4)
	name, end := chars(b, 15, cch, b[14]&0x01 != 0)
	n.builtin = n.flags&nameBuiltin != 0
	if n.builtin && name != "" {
		if c := int(name[0]); c < len(builtinNames) {
			name = builtinNames[c]
		}
	}
	n.name = name
	end = min(end, len(b))
	n.rgce = b[end:min(end+cce, len(b))]
	n.extra = b[min(end+cce, len(b)):]
	return n
}

// definedNames decodes the user-visible NAME records into wb.Names.
// Built-in names (Print_Area ...), hidden names and function names are
// skipped, as the .xlsx reader skips them.
func (p *parser) definedNames() {
	sheetIndex := map[int]int{} // BOUNDSHEET position (1-based) -> model sheet
	for i, bs := range p.sheets {
		if bs.kind == 0 {
			sheetIndex[i+1] = len(sheetIndex)
		}
	}
	for _, n := range p.names {
		if n.builtin || n.flags&(nameHidden|nameFunc|nameVB|nameProc) != 0 || n.name == "" {
			continue
		}
		dn := model.DefinedName{Name: n.name, Sheet: -1}
		if n.itab != 0 {
			idx, ok := sheetIndex[n.itab]
			if !ok {
				continue
			}
			dn.Sheet = idx
		}
		if len(n.rgce) == 0 {
			continue // a name used in formulas but never defined
		}
		f, err := p.decodeFormula(n.rgce, n.extra, 0, 0, true)
		if err != nil {
			p.warnf("defined name %s: %v", n.name, err)
			continue
		}
		dn.Formula = f
		p.wb.Names = append(p.wb.Names, dn)
	}
}
