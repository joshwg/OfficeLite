package docfile

import (
	"encoding/binary"
	"strings"

	"officelite/common/colors"
)

// Single property modifiers ([MS-DOC] 2.6) used by this package.
const (
	// character
	sprmCFRMarkDel   = 0x0800
	sprmCFRMarkIns   = 0x0801
	sprmCFFldVanish  = 0x0802
	sprmCPicLocation = 0x6A03
	sprmCFData       = 0x0806
	sprmCFOle2       = 0x080A
	sprmCHighlight   = 0x2A0C
	sprmCIstd        = 0x4A30
	sprmCFBold       = 0x0835
	sprmCFItalic     = 0x0836
	sprmCFStrike     = 0x0837
	sprmCFSmallCaps  = 0x083A
	sprmCFCaps       = 0x083B
	sprmCFVanish     = 0x083C
	sprmCFtcDefault  = 0x4A3D
	sprmCKul         = 0x2A3E
	sprmCIco         = 0x2A42
	sprmCHps         = 0x4A43
	sprmCIss         = 0x2A48
	sprmCRgFtc0      = 0x4A4F
	sprmCRgFtc1      = 0x4A50
	sprmCRgFtc2      = 0x4A51
	sprmCFSpec       = 0x0855
	sprmCFtcBi       = 0x4A5E
	sprmCShd80       = 0x4866
	sprmCCv          = 0x6870
	sprmCShd         = 0xCA71

	// paragraph
	sprmPIstd               = 0x4600
	sprmPJc80               = 0x2403
	sprmPFKeep              = 0x2405
	sprmPFKeepFollow        = 0x2406
	sprmPFPageBreakBefore   = 0x2407
	sprmPIlvl               = 0x260A
	sprmPIlfo               = 0x460B
	sprmPChgTabsPapx        = 0xC60D
	sprmPDxaRight80         = 0x840E
	sprmPDxaLeft80          = 0x840F
	sprmPDxaLeft180         = 0x8411
	sprmPDyaLine            = 0x6412
	sprmPDyaBefore          = 0xA413
	sprmPDyaAfter           = 0xA414
	sprmPChgTabs            = 0xC615
	sprmPFInTable           = 0x2416
	sprmPFTtp               = 0x2417
	sprmPShd80              = 0x442D
	sprmPFWidowControl      = 0x2431
	sprmPOutLvl             = 0x2640
	sprmPHugePapx           = 0x6646
	sprmPItap               = 0x6649
	sprmPDtap               = 0x664A
	sprmPFInnerTableCell    = 0x244B
	sprmPFInnerTtp          = 0x244C
	sprmPShd                = 0xC64D
	sprmPDxaRight           = 0x845D
	sprmPDxaLeft            = 0x845E
	sprmPDxaLeft1           = 0x8460
	sprmPJc                 = 0x2461
	sprmPFContextualSpacing = 0x246D

	// table
	sprmTJc90            = 0x5400
	sprmTDxaLeft         = 0x9601
	sprmTDxaGapHalf      = 0x9602
	sprmTFCantSplit      = 0x3403
	sprmTTableHeader     = 0x3404
	sprmTFCantSplit90    = 0x3466
	sprmTTableBorders80  = 0xD605
	sprmTDyaRowHeight    = 0x9407
	sprmTDefTable        = 0xD608
	sprmTDefTableShd80   = 0xD609
	sprmTDefTableShd3rd  = 0xD60C
	sprmTDefTableShd     = 0xD612
	sprmTTableBorders    = 0xD613
	sprmTDefTableShd2nd  = 0xD616
	sprmTVertMerge       = 0xD62B
	sprmTVertAlign       = 0xD62C
	sprmTJc              = 0x548A
	sprmTIstd            = 0x563A
	sprmTDefTableShdRaw  = 0xD670
	sprmTDefTableShdRaw2 = 0xD671
	sprmTDefTableShdRaw3 = 0xD672

	// section
	sprmSBOrientation = 0x301D
	sprmSXaPage       = 0xB01F
	sprmSYaPage       = 0xB020
	sprmSDxaLeft      = 0xB021
	sprmSDxaRight     = 0xB022
	sprmSDyaTop       = 0x9023
	sprmSDyaBottom    = 0x9024
	sprmSDyaHdrTop    = 0xB017
	sprmSDyaHdrBottom = 0xB018
)

// sprmOperandSize returns the size of the operand of sprm op that starts at
// b (which holds the rest of the grpprl), including any size prefix, or -1.
func sprmOperandSize(op uint16, b []byte) int {
	switch op >> 13 {
	case 0, 1:
		return 1
	case 2, 4, 5:
		return 2
	case 3:
		return 4
	case 7:
		return 3
	}
	// spra 6: variable length
	switch op {
	case sprmTDefTable, 0xD606: // 2-byte cb, counting itself minus one
		if len(b) < 2 {
			return -1
		}
		return int(binary.LittleEndian.Uint16(b)) + 1
	case sprmPChgTabs:
		if len(b) < 1 {
			return -1
		}
		if b[0] != 255 {
			return 1 + int(b[0])
		}
		// PChgTabsDelClose + PChgTabsAdd with the size derived from the counts.
		if len(b) < 2 {
			return -1
		}
		n := 2 + 4*int(b[1])
		if len(b) < n+1 {
			return -1
		}
		return n + 1 + 3*int(b[n])
	}
	if len(b) < 1 {
		return -1
	}
	return 1 + int(b[0])
}

// eachSprm calls f for each sprm in grpprl with its operand. For
// variable-length sprms the operand excludes the size prefix (1 byte, or 2
// for sprmTDefTable).
func eachSprm(grpprl []byte, f func(op uint16, arg []byte)) {
	for len(grpprl) >= 2 {
		op := binary.LittleEndian.Uint16(grpprl)
		grpprl = grpprl[2:]
		n := sprmOperandSize(op, grpprl)
		if n < 0 || n > len(grpprl) {
			return
		}
		arg := grpprl[:n]
		if op>>13 == 6 {
			switch op {
			case sprmTDefTable, 0xD606:
				arg = arg[2:]
			default:
				arg = arg[1:]
			}
		}
		f(op, arg)
		grpprl = grpprl[n:]
	}
}

func u16(b []byte) uint16 {
	if len(b) < 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func i16(b []byte) int16 { return int16(u16(b)) }

func u32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func u8(b []byte) byte {
	if len(b) < 1 {
		return 0
	}
	return b[0]
}

// grpprl builds a list of sprms.
type grpprl []byte

func (g *grpprl) op(op uint16) { *g = binary.LittleEndian.AppendUint16(*g, op) }

func (g *grpprl) b(op uint16, v byte) { g.op(op); *g = append(*g, v) }

func (g *grpprl) w(op uint16, v uint16) {
	g.op(op)
	*g = binary.LittleEndian.AppendUint16(*g, v)
}

func (g *grpprl) l(op uint16, v uint32) {
	g.op(op)
	*g = binary.LittleEndian.AppendUint32(*g, v)
}

// v appends a variable-length sprm with a one-byte size prefix.
func (g *grpprl) v(op uint16, data []byte) {
	g.op(op)
	*g = append(*g, byte(len(data)))
	*g = append(*g, data...)
}

// Colors.

// icoColors maps an Ico (sprmCIco, highlight) to RRGGBB; 0 is automatic.
var icoColors = [...]string{"", "000000", "0000FF", "00FFFF", "00FF00", "FF00FF", "FF0000", "FFFF00",
	"FFFFFF", "000080", "008080", "008000", "800080", "800000", "808000", "808080", "C0C0C0"}

// highlightNames maps an Ico to the docx highlight name.
var highlightNames = [...]string{"", "black", "blue", "cyan", "green", "magenta", "red", "yellow",
	"white", "darkBlue", "darkCyan", "darkGreen", "darkMagenta", "darkRed", "darkYellow", "darkGray", "lightGray"}

func icoColor(ico byte) string {
	if int(ico) < len(icoColors) {
		return icoColors[ico]
	}
	return ""
}

// highlightIco maps a highlight name (or RRGGBB matching an Ico color) to an Ico.
func highlightIco(h string) (byte, bool) {
	for i, n := range highlightNames {
		if i > 0 && strings.EqualFold(n, h) {
			return byte(i), true
		}
	}
	return 0, false
}

// colorRef decodes a COLORREF (red, green, blue, fAuto); "" for automatic.
func colorRef(b []byte) string {
	if len(b) < 4 || b[3] == 0xFF {
		return ""
	}
	return colors.Hex(colors.RGBA(b[0], b[1], b[2], 255))
}

// encColorRef encodes RRGGBB as a COLORREF ("" → automatic).
func encColorRef(hex string) [4]byte {
	c, ok := colors.ParseHex(hex)
	if !ok {
		return [4]byte{0, 0, 0, 0xFF}
	}
	return [4]byte{c.R, c.G, c.B, 0}
}

// shdColor returns the visible background of a Shd (cvFore, cvBack, ipat).
func shdColor(b []byte) string {
	if len(b) < 10 {
		return ""
	}
	switch u16(b[8:]) {
	case 0: // clear
		return colorRef(b[4:8])
	case 1: // solid
		return colorRef(b[0:4])
	case 0xFFFF: // nil
		return ""
	}
	return colorRef(b[4:8])
}

// shd80Color decodes a SHD80 (icoFore:5, icoBack:5, ipat:6).
func shd80Color(v uint16) string {
	fore, back, ipat := byte(v&0x1F), byte(v>>5&0x1F), v>>10
	switch ipat {
	case 0:
		return icoColor(back)
	case 1:
		return icoColor(fore)
	}
	return icoColor(back)
}

// encShd encodes a clear Shd with background hex.
func encShd(hex string) []byte {
	b := make([]byte, 10)
	fore := [4]byte{0, 0, 0, 0xFF}
	back := encColorRef(hex)
	copy(b, fore[:])
	copy(b[4:], back[:])
	return b
}

func twips(v int) float64 { return float64(v) / 20 }

func toTwips(pt float64) int {
	if pt < 0 {
		return -int(-pt*20 + 0.5)
	}
	return int(pt*20 + 0.5)
}
