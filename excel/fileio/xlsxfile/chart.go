package xlsxfile

import (
	"bytes"
	"encoding/xml"
	"image/color"
	"io"
	"path"
	"strconv"
	"strings"

	"officelite/common/colors"
	"officelite/excel/model"
)

// Reading charts: a worksheet's <drawing> points at a DrawingML drawing
// (xl/drawings/drawingN.xml) whose anchors hold graphic frames pointing
// at chart parts (xl/charts/chartN.xml); a chart sheet
// (xl/chartsheets/sheetN.xml) has a drawing with one chart. The chart
// part is decoded into model.Chart with colours resolved against the
// workbook's theme, and kept as read (model.ChartRaw) so that an
// unedited chart is saved as it came.

const (
	relDrawing    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing"
	relChart      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart"
	relChartsheet = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/chartsheet"
	nsChart       = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	nsChartStrict = "http://purl.oclc.org/ooxml/drawingml/chart"
)

// --- XML structures (matched by local name, so transitional and strict
// files read alike) ---

type xVal struct {
	Val *string `xml:"val,attr"`
}

// xBool is a CT_Boolean: present without val means true.
type xBool struct {
	Val *string `xml:"val,attr"`
}

func (b *xBool) on(def bool) bool {
	if b == nil {
		return def
	}
	if b.Val == nil {
		return true
	}
	return *b.Val == "1" || *b.Val == "true"
}

func (v *xVal) str(def string) string {
	if v == nil || v.Val == nil {
		return def
	}
	return *v.Val
}

func (v *xVal) int(def int) int {
	if v == nil || v.Val == nil {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSuffix(*v.Val, "%"))
	if err != nil {
		return def
	}
	return n
}

func (v *xVal) float() (float64, bool) {
	if v == nil || v.Val == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(*v.Val, 64)
	return f, err == nil
}

type xClr struct {
	XMLName xml.Name
	Val     string `xml:"val,attr"`
	LastClr string `xml:"lastClr,attr"`
	Mods    []struct {
		XMLName xml.Name
		Val     string `xml:"val,attr"`
	} `xml:",any"`
}

type xColor struct {
	Clrs []xClr `xml:",any"`
}

type xGradFill struct {
	Stops []struct {
		Pos  int    `xml:"pos,attr"`
		Clrs []xClr `xml:",any"`
	} `xml:"gsLst>gs"`
}

type xLn struct {
	W         *int       `xml:"w,attr"`
	NoFill    *struct{}  `xml:"noFill"`
	SolidFill *xColor    `xml:"solidFill"`
	GradFill  *xGradFill `xml:"gradFill"`
	PrstDash  *xVal      `xml:"prstDash"`
}

type xSpPr struct {
	NoFill    *struct{}  `xml:"noFill"`
	SolidFill *xColor    `xml:"solidFill"`
	GradFill  *xGradFill `xml:"gradFill"`
	PattFill  *struct {
		Fg xColor `xml:"fgClr"`
	} `xml:"pattFill"`
	Ln *xLn `xml:"ln"`
}

type xRPr struct {
	Sz        *float64 `xml:"sz,attr"`
	B         *string  `xml:"b,attr"`
	I         *string  `xml:"i,attr"`
	SolidFill *xColor  `xml:"solidFill"`
	Latin     *struct {
		Typeface string `xml:"typeface,attr"`
	} `xml:"latin"`
}

type xBodyPr struct {
	Rot  *int   `xml:"rot,attr"`
	Vert string `xml:"vert,attr"`
}

type xPara struct {
	PPr struct {
		DefRPr *xRPr `xml:"defRPr"`
	} `xml:"pPr"`
	Runs []struct {
		XMLName xml.Name
		RPr     *xRPr  `xml:"rPr"`
		T       string `xml:"t"`
	} `xml:",any"`
}

type xTxPr struct {
	BodyPr *xBodyPr `xml:"bodyPr"`
	P      []xPara  `xml:"p"`
}

type xPt struct {
	Idx int    `xml:"idx,attr"`
	V   string `xml:"v"`
}

type xCache struct {
	FormatCode string `xml:"formatCode"`
	PtCount    xVal   `xml:"ptCount"`
	Pts        []xPt  `xml:"pt"`
	Lvls       []struct {
		Pts []xPt `xml:"pt"`
	} `xml:"lvl"`
}

type xDataRef struct {
	F                string  `xml:"f"`
	NumCache         *xCache `xml:"numCache"`
	StrCache         *xCache `xml:"strCache"`
	MultiLvlStrCache *xCache `xml:"multiLvlStrCache"`
}

type xDataSrc struct {
	NumRef         *xDataRef `xml:"numRef"`
	StrRef         *xDataRef `xml:"strRef"`
	MultiLvlStrRef *xDataRef `xml:"multiLvlStrRef"`
	NumLit         *xCache   `xml:"numLit"`
	StrLit         *xCache   `xml:"strLit"`
}

type xTx struct {
	StrRef *xDataRef `xml:"strRef"`
	V      string    `xml:"v"`
	Rich   *xTxPr    `xml:"rich"`
}

type xDLbls struct {
	Delete *xBool `xml:"delete"`
	NumFmt *struct {
		Code string `xml:"formatCode,attr"`
	} `xml:"numFmt"`
	SpPr          *xSpPr `xml:"spPr"`
	TxPr          *xTxPr `xml:"txPr"`
	DLblPos       *xVal  `xml:"dLblPos"`
	ShowLegendKey *xBool `xml:"showLegendKey"`
	ShowVal       *xBool `xml:"showVal"`
	ShowCatName   *xBool `xml:"showCatName"`
	ShowSerName   *xBool `xml:"showSerName"`
	ShowPercent   *xBool `xml:"showPercent"`
	Separator     string `xml:"separator"`
	ShowLeader    *xBool `xml:"showLeaderLines"`
}

type xMarker struct {
	Symbol *xVal  `xml:"symbol"`
	Size   *xVal  `xml:"size"`
	SpPr   *xSpPr `xml:"spPr"`
}

type xSer struct {
	Idx       xVal     `xml:"idx"`
	Order     xVal     `xml:"order"`
	Tx        *xTx     `xml:"tx"`
	SpPr      *xSpPr   `xml:"spPr"`
	InvertNeg *xBool   `xml:"invertIfNegative"`
	Marker    *xMarker `xml:"marker"`
	Explosion *xVal    `xml:"explosion"`
	DPts      []struct {
		Idx       xVal   `xml:"idx"`
		SpPr      *xSpPr `xml:"spPr"`
		Explosion *xVal  `xml:"explosion"`
	} `xml:"dPt"`
	DLbls  *xDLbls   `xml:"dLbls"`
	Cat    *xDataSrc `xml:"cat"`
	Val    *xDataSrc `xml:"val"`
	XVal   *xDataSrc `xml:"xVal"`
	YVal   *xDataSrc `xml:"yVal"`
	Smooth *xBool    `xml:"smooth"`
}

type xTitle struct {
	Tx      *xTx   `xml:"tx"`
	Overlay *xBool `xml:"overlay"`
	SpPr    *xSpPr `xml:"spPr"`
	TxPr    *xTxPr `xml:"txPr"`
}

// xPlotItem is any child of c:plotArea: a chart group or an axis.
type xPlotItem struct {
	XMLName       xml.Name
	BarDir        *xVal   `xml:"barDir"`
	Grouping      *xVal   `xml:"grouping"`
	VaryColors    *xBool  `xml:"varyColors"`
	Ser           []xSer  `xml:"ser"`
	DLbls         *xDLbls `xml:"dLbls"`
	GapWidth      *xVal   `xml:"gapWidth"`
	Overlap       *xVal   `xml:"overlap"`
	HoleSize      *xVal   `xml:"holeSize"`
	FirstSliceAng *xVal   `xml:"firstSliceAng"`
	ScatterStyle  *xVal   `xml:"scatterStyle"`
	Marker        *xBool  `xml:"marker"`
	AxID          []xVal  `xml:"axId"`

	// Axes.
	Scaling *struct {
		LogBase     *xVal `xml:"logBase"`
		Orientation *xVal `xml:"orientation"`
		Max         *xVal `xml:"max"`
		Min         *xVal `xml:"min"`
	} `xml:"scaling"`
	Delete         *xBool `xml:"delete"`
	AxPos          *xVal  `xml:"axPos"`
	MajorGridlines *struct {
		SpPr *xSpPr `xml:"spPr"`
	} `xml:"majorGridlines"`
	MinorGridlines *struct {
		SpPr *xSpPr `xml:"spPr"`
	} `xml:"minorGridlines"`
	Title  *xTitle `xml:"title"`
	NumFmt *struct {
		Code         string `xml:"formatCode,attr"`
		SourceLinked string `xml:"sourceLinked,attr"`
	} `xml:"numFmt"`
	MajorTickMark *xVal  `xml:"majorTickMark"`
	MinorTickMark *xVal  `xml:"minorTickMark"`
	TickLblPos    *xVal  `xml:"tickLblPos"`
	SpPr          *xSpPr `xml:"spPr"`
	TxPr          *xTxPr `xml:"txPr"`
	CrossAx       *xVal  `xml:"crossAx"`
	Crosses       *xVal  `xml:"crosses"`
	CrossesAt     *xVal  `xml:"crossesAt"`
	CrossBetween  *xVal  `xml:"crossBetween"`
	MajorUnit     *xVal  `xml:"majorUnit"`
	MinorUnit     *xVal  `xml:"minorUnit"`
	TickLblSkip   *xVal  `xml:"tickLblSkip"`
	TickMarkSkip  *xVal  `xml:"tickMarkSkip"`
	NoMultiLvlLbl *xBool `xml:"noMultiLvlLbl"`
}

type xChartSpace struct {
	RoundedCorners *xBool `xml:"roundedCorners"`
	Style          *xVal  `xml:"style"`
	Alt            []struct {
		Fallback struct {
			Style *xVal `xml:"style"`
		} `xml:"Fallback"`
	} `xml:"AlternateContent"`
	PivotSource *struct {
		Name string `xml:"name"`
	} `xml:"pivotSource"`
	Chart struct {
		Title            *xTitle `xml:"title"`
		AutoTitleDeleted *xBool  `xml:"autoTitleDeleted"`
		PlotArea         struct {
			Items []xPlotItem `xml:",any"`
		} `xml:"plotArea"`
		Legend *struct {
			LegendPos *xVal `xml:"legendPos"`
			Entries   []struct {
				Idx    xVal   `xml:"idx"`
				Delete *xBool `xml:"delete"`
			} `xml:"legendEntry"`
			Overlay *xBool `xml:"overlay"`
			SpPr    *xSpPr `xml:"spPr"`
			TxPr    *xTxPr `xml:"txPr"`
		} `xml:"legend"`
		PlotVisOnly  *xBool `xml:"plotVisOnly"`
		DispBlanksAs *xVal  `xml:"dispBlanksAs"`
	} `xml:"chart"`
	SpPr *xSpPr `xml:"spPr"`
	TxPr *xTxPr `xml:"txPr"`
}

// --- the theme ---

// readTheme reads the workbook's theme part.
func (r *reader) readTheme(name string) *model.Theme {
	rc, err := r.open(name)
	if err != nil {
		return nil
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil
	}
	var th struct {
		Scheme struct {
			Items []struct {
				XMLName xml.Name
				Clrs    []xClr `xml:",any"`
			} `xml:",any"`
		} `xml:"themeElements>clrScheme"`
		Major struct {
			Typeface string `xml:"typeface,attr"`
		} `xml:"themeElements>fontScheme>majorFont>latin"`
		Minor struct {
			Typeface string `xml:"typeface,attr"`
		} `xml:"themeElements>fontScheme>minorFont>latin"`
	}
	if err := xml.Unmarshal(data, &th); err != nil {
		return nil
	}
	t := model.DefaultTheme()
	t.XML = data
	order := []string{"dk1", "lt1", "dk2", "lt2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"}
	for _, it := range th.Scheme.Items {
		for i, n := range order {
			if it.XMLName.Local == n && len(it.Clrs) > 0 {
				if c, ok := resolveClr(nil, it.Clrs[0]); ok {
					t.Colors[i] = c
				}
			}
		}
	}
	if th.Major.Typeface != "" {
		t.MajorFont = th.Major.Typeface
	}
	if th.Minor.Typeface != "" {
		t.MinorFont = th.Minor.Typeface
	}
	return t
}

// --- colours ---

var presetColors = map[string]uint32{
	"black": 0x000000, "white": 0xFFFFFF, "red": 0xFF0000, "green": 0x008000, "blue": 0x0000FF,
	"yellow": 0xFFFF00, "gray": 0x808080, "grey": 0x808080, "orange": 0xFFA500, "purple": 0x800080,
	"darkBlue": 0x00008B, "darkRed": 0x8B0000, "darkGreen": 0x006400, "lightGray": 0xD3D3D3,
}

// resolveClr turns a DrawingML colour element (a:srgbClr, a:schemeClr,
// a:sysClr, a:prstClr, with lumMod, lumOff, tint, shade and alpha) into
// RGB. A scheme colour needs th; phClr (a style's placeholder) fails.
func resolveClr(th *model.Theme, c xClr) (color.NRGBA, bool) {
	var out color.NRGBA
	switch c.XMLName.Local {
	case "srgbClr":
		v, ok := colors.ParseHex(c.Val)
		if !ok {
			return out, false
		}
		out = v
	case "sysClr":
		v, ok := colors.ParseHex(c.LastClr)
		if !ok {
			v = map[string]color.NRGBA{"window": colors.White, "windowText": {A: 255}}[c.Val]
			if v.A == 0 {
				return out, false
			}
		}
		out = v
	case "prstClr":
		v, ok := presetColors[c.Val]
		if !ok {
			return out, false
		}
		out = colors.RGB(v)
	case "schemeClr":
		if th == nil {
			th = model.DefaultTheme()
		}
		idx := map[string]int{"dk1": 0, "tx1": 0, "lt1": 1, "bg1": 1, "dk2": 2, "tx2": 2, "lt2": 3, "bg2": 3,
			"accent1": 4, "accent2": 5, "accent3": 6, "accent4": 7, "accent5": 8, "accent6": 9, "hlink": 10, "folHlink": 11}
		i, ok := idx[c.Val]
		if !ok {
			return out, false
		}
		out = th.Colors[i]
	case "scrgbClr", "hslClr":
		return out, false
	default:
		return out, false
	}
	out.A = 255
	lumMod, lumOff := 100000, 0
	for _, m := range c.Mods {
		v, _ := strconv.Atoi(m.Val)
		switch m.XMLName.Local {
		case "lumMod":
			lumMod = v
		case "lumOff":
			lumOff = v
		case "tint":
			f := float64(v) / 100000
			mix := func(x uint8) uint8 { return uint8(255 - (255-float64(x))*f + 0.5) }
			out = color.NRGBA{R: mix(out.R), G: mix(out.G), B: mix(out.B), A: out.A}
		case "shade":
			f := float64(v) / 100000
			mix := func(x uint8) uint8 { return uint8(float64(x)*f + 0.5) }
			out = color.NRGBA{R: mix(out.R), G: mix(out.G), B: mix(out.B), A: out.A}
		case "alpha":
			out.A = uint8(min(max(v, 0), 100000) * 255 / 100000)
		}
	}
	a := out.A
	out = model.LumMod(out, lumMod, lumOff)
	out.A = a
	return out, true
}

// chartReader converts one chart part's XML.
type chartReader struct {
	th *model.Theme
}

func (cr *chartReader) color(c *xColor) (model.Paint, bool) {
	if c == nil || len(c.Clrs) == 0 {
		return model.Paint{}, false
	}
	v, ok := resolveClr(cr.th, c.Clrs[0])
	if !ok {
		return model.Paint{}, false
	}
	return model.Paint{Kind: model.FillSolid, Color: v}, true
}

// grad approximates a gradient by its middle stop.
func (cr *chartReader) grad(g *xGradFill) (model.Paint, bool) {
	if g == nil || len(g.Stops) == 0 || len(g.Stops[len(g.Stops)/2].Clrs) == 0 {
		return model.Paint{}, false
	}
	v, ok := resolveClr(cr.th, g.Stops[len(g.Stops)/2].Clrs[0])
	return model.Paint{Kind: model.FillSolid, Color: v}, ok
}

func (cr *chartReader) shape(sp *xSpPr) model.ShapeProps {
	var out model.ShapeProps
	if sp == nil {
		return out
	}
	switch {
	case sp.NoFill != nil:
		out.Fill.Kind = model.FillNone
	case sp.SolidFill != nil:
		out.Fill, _ = cr.color(sp.SolidFill)
	case sp.GradFill != nil:
		out.Fill, _ = cr.grad(sp.GradFill)
	case sp.PattFill != nil:
		out.Fill, _ = cr.color(&sp.PattFill.Fg)
	}
	if ln := sp.Ln; ln != nil {
		switch {
		case ln.NoFill != nil:
			out.Line.Kind = model.FillNone
		case ln.SolidFill != nil:
			out.Line, _ = cr.color(ln.SolidFill)
		case ln.GradFill != nil:
			out.Line, _ = cr.grad(ln.GradFill)
		}
		if ln.W != nil {
			out.Width = float32(*ln.W) / 12700
		}
		out.Dash = ln.PrstDash.str("")
	}
	return out
}

func (cr *chartReader) rPr(rp *xRPr, t *model.TextProps) {
	if rp == nil {
		return
	}
	if rp.Sz != nil {
		t.SizePt = float32(*rp.Sz / 100)
	}
	if rp.B != nil {
		b := *rp.B == "1" || *rp.B == "true"
		t.Bold = &b
	}
	if rp.I != nil {
		t.Italic = *rp.I == "1" || *rp.I == "true"
	}
	if p, ok := cr.color(rp.SolidFill); ok {
		t.Color = p
	}
	if rp.Latin != nil && !strings.HasPrefix(rp.Latin.Typeface, "+") {
		t.Font = rp.Latin.Typeface
	}
}

func (cr *chartReader) text(tx *xTxPr) model.TextProps {
	var t model.TextProps
	if tx == nil {
		return t
	}
	if tx.BodyPr != nil && tx.BodyPr.Rot != nil && *tx.BodyPr.Rot != -60000000 {
		t.Rotation, t.HasRot = *tx.BodyPr.Rot/60000, true
	}
	if tx.BodyPr != nil && (tx.BodyPr.Vert == "vert270" || tx.BodyPr.Vert == "vert") {
		t.Rotation, t.HasRot = -90, true
		if tx.BodyPr.Vert == "vert" {
			t.Rotation = 90
		}
	}
	for _, p := range tx.P {
		cr.rPr(p.PPr.DefRPr, &t)
	}
	return t
}

func cachePoints(c *xCache, num bool) []model.ChartValue {
	if c == nil {
		return nil
	}
	n := c.PtCount.int(0)
	for _, p := range c.Pts {
		n = max(n, p.Idx+1)
	}
	n = min(n, 1<<20)
	out := make([]model.ChartValue, n)
	for _, p := range c.Pts {
		if p.Idx < 0 || p.Idx >= n {
			continue
		}
		if num {
			if v, err := strconv.ParseFloat(strings.TrimSpace(p.V), 64); err == nil {
				out[p.Idx] = model.ChartValue{Kind: model.ValueNumber, Num: v, Text: p.V}
				continue
			}
		}
		out[p.Idx] = model.ChartValue{Kind: model.ValueText, Text: p.V}
	}
	return out
}

func (cr *chartReader) data(src *xDataSrc) model.ChartData {
	var d model.ChartData
	if src == nil {
		return d
	}
	switch {
	case src.NumRef != nil:
		d.Ref = strings.TrimSpace(src.NumRef.F)
		d.Cache = cachePoints(src.NumRef.NumCache, true)
		if src.NumRef.NumCache != nil {
			d.Format = src.NumRef.NumCache.FormatCode
		}
	case src.StrRef != nil:
		d.Ref, d.Text = strings.TrimSpace(src.StrRef.F), true
		d.Cache = cachePoints(src.StrRef.StrCache, false)
	case src.MultiLvlStrRef != nil:
		d.Ref, d.Text = strings.TrimSpace(src.MultiLvlStrRef.F), true
		if c := src.MultiLvlStrRef.MultiLvlStrCache; c != nil {
			for i, l := range c.Lvls {
				lc := &xCache{PtCount: c.PtCount, Pts: l.Pts}
				if i == 0 {
					d.Cache = cachePoints(lc, false)
				} else {
					d.Levels = append(d.Levels, cachePoints(lc, false))
				}
			}
		}
	case src.NumLit != nil:
		d.Cache, d.Format = cachePoints(src.NumLit, true), src.NumLit.FormatCode
	case src.StrLit != nil:
		d.Cache, d.Text = cachePoints(src.StrLit, false), true
	}
	return d
}

func (cr *chartReader) tx(tx *xTx) model.ChartData {
	var d model.ChartData
	if tx == nil {
		return d
	}
	if tx.StrRef != nil {
		d.Ref, d.Text = strings.TrimSpace(tx.StrRef.F), true
		d.Cache = cachePoints(tx.StrRef.StrCache, false)
	} else {
		d.Literal = tx.V
	}
	return d
}

func (cr *chartReader) labels(dl *xDLbls) *model.DataLabels {
	if dl == nil {
		return nil
	}
	l := &model.DataLabels{
		Delete:      dl.Delete.on(false),
		ShowVal:     dl.ShowVal.on(false),
		ShowPercent: dl.ShowPercent.on(false),
		ShowCat:     dl.ShowCatName.on(false),
		ShowSer:     dl.ShowSerName.on(false),
		ShowKey:     dl.ShowLegendKey.on(false),
		ShowLeader:  dl.ShowLeader.on(false),
		Pos:         dl.DLblPos.str(""),
		Separator:   dl.Separator,
		Font:        cr.text(dl.TxPr),
		Shape:       cr.shape(dl.SpPr),
	}
	if dl.NumFmt != nil {
		l.NumFmt = dl.NumFmt.Code
	}
	return l
}

func (cr *chartReader) title(t *xTitle) *model.ChartTitle {
	if t == nil {
		return nil
	}
	out := &model.ChartTitle{Overlay: t.Overlay.on(false), Shape: cr.shape(t.SpPr), Font: cr.text(t.TxPr)}
	if t.Tx != nil {
		if t.Tx.Rich != nil {
			var lines []string
			for _, p := range t.Tx.Rich.P {
				var b strings.Builder
				for _, r := range p.Runs {
					if r.XMLName.Local == "r" || r.XMLName.Local == "fld" {
						b.WriteString(r.T)
						if r.RPr != nil {
							cr.rPr(r.RPr, &out.Font)
						}
					}
				}
				lines = append(lines, b.String())
				cr.rPr(p.PPr.DefRPr, &out.Font)
			}
			out.Text = strings.Join(lines, "\n")
			if t.Tx.Rich.BodyPr != nil && t.Tx.Rich.BodyPr.Rot != nil && *t.Tx.Rich.BodyPr.Rot != -60000000 {
				out.Font.Rotation, out.Font.HasRot = *t.Tx.Rich.BodyPr.Rot/60000, true
			}
		} else {
			out.Ref = cr.tx(t.Tx)
		}
	}
	return out
}

// parseChart decodes a chart part.
func parseChart(data []byte, th *model.Theme) (*model.Chart, error) {
	var cs xChartSpace
	if err := xml.Unmarshal(data, &cs); err != nil {
		return nil, err
	}
	cr := &chartReader{th: th}
	c := &model.Chart{
		RoundedCorners:   cs.RoundedCorners.on(false),
		Style:            cs.Style.int(2),
		Title:            cr.title(cs.Chart.Title),
		AutoTitleDeleted: cs.Chart.AutoTitleDeleted.on(false),
		PlotVisOnly:      cs.Chart.PlotVisOnly.on(true),
		ChartArea:        cr.shape(cs.SpPr),
		Font:             cr.text(cs.TxPr),
	}
	if cs.PivotSource != nil {
		c.PivotSource = cs.PivotSource.Name
	}
	for _, a := range cs.Alt {
		if a.Fallback.Style != nil {
			c.Style = a.Fallback.Style.int(c.Style)
		}
	}
	switch cs.Chart.DispBlanksAs.str("gap") {
	case "zero":
		c.DispBlanks = model.BlanksZero
	case "span":
		c.DispBlanks = model.BlanksSpan
	}
	if lg := cs.Chart.Legend; lg != nil {
		c.Legend = &model.ChartLegend{Pos: lg.LegendPos.str("r"), Overlay: lg.Overlay.on(false), Shape: cr.shape(lg.SpPr), Font: cr.text(lg.TxPr)}
		for _, e := range lg.Entries {
			if e.Delete.on(false) {
				c.Legend.Deleted = append(c.Legend.Deleted, e.Idx.int(0))
			}
		}
	}
	for i := range cs.Chart.PlotArea.Items {
		it := &cs.Chart.PlotArea.Items[i]
		name := it.XMLName.Local
		switch {
		case name == "spPr":
			// c:plotArea's own format: decoded again below.
		case strings.HasSuffix(name, "Chart"):
			c.Plots = append(c.Plots, cr.plot(it))
		case strings.HasSuffix(name, "Ax"):
			c.Axes = append(c.Axes, cr.axis(it))
		}
	}
	// The plot area's spPr: a direct child of c:plotArea.
	var pa struct {
		SpPr *xSpPr `xml:"chart>plotArea>spPr"`
	}
	if xml.Unmarshal(data, &pa) == nil {
		c.PlotArea = cr.shape(pa.SpPr)
	}
	return c, nil
}

func (cr *chartReader) plot(it *xPlotItem) model.ChartPlot {
	p := model.ChartPlot{
		VaryColors: it.VaryColors.on(false),
		GapWidth:   it.GapWidth.int(150),
		Overlap:    it.Overlap.int(0),
		HoleSize:   it.HoleSize.int(50),
		FirstSlice: it.FirstSliceAng.int(0),
		Marker:     it.Marker.on(false),
		Labels:     cr.labels(it.DLbls),
	}
	switch it.XMLName.Local {
	case "barChart":
		p.Type = model.ChartBar
		p.Horizontal = it.BarDir.str("col") == "bar"
	case "lineChart":
		p.Type = model.ChartLine
	case "pieChart":
		p.Type = model.ChartPie
	case "doughnutChart":
		p.Type = model.ChartDoughnut
	case "areaChart":
		p.Type = model.ChartArea
	case "scatterChart":
		p.Type = model.ChartScatter
		p.ScatterStyle = it.ScatterStyle.str("marker")
	default:
		p.Type, p.OtherKind = model.ChartOther, it.XMLName.Local
	}
	switch it.Grouping.str("") {
	case "stacked":
		p.Grouping = model.GroupStacked
	case "percentStacked":
		p.Grouping = model.GroupPercent
	}
	for _, a := range it.AxID {
		p.AxisIDs = append(p.AxisIDs, a.int(0))
	}
	for i := range it.Ser {
		p.Series = append(p.Series, cr.series(&it.Ser[i], p.Type))
	}
	return p
}

func (cr *chartReader) series(x *xSer, t model.ChartType) model.ChartSeries {
	s := model.ChartSeries{
		Idx:       x.Idx.int(0),
		Order:     x.Order.int(x.Idx.int(0)),
		Name:      cr.tx(x.Tx),
		Shape:     cr.shape(x.SpPr),
		InvertNeg: x.InvertNeg.on(false),
		Explosion: x.Explosion.int(0),
		Labels:    cr.labels(x.DLbls),
		// A scatter series without c:smooth is smooth (CT_Boolean's
		// default); lines are not.
		Smooth: x.Smooth.on(false),
	}
	if m := x.Marker; m != nil {
		s.Marker = model.Marker{Symbol: m.Symbol.str(""), Size: m.Size.int(0), Shape: cr.shape(m.SpPr)}
	}
	for _, d := range x.DPts {
		s.Points = append(s.Points, model.DataPoint{Idx: d.Idx.int(0), Shape: cr.shape(d.SpPr), Explosion: d.Explosion.int(0)})
	}
	if t == model.ChartScatter {
		s.Cat, s.Val = cr.data(x.XVal), cr.data(x.YVal)
	} else {
		s.Cat, s.Val = cr.data(x.Cat), cr.data(x.Val)
	}
	return s
}

func (cr *chartReader) axis(it *xPlotItem) model.ChartAxis {
	a := model.ChartAxis{
		ID:           it.AxID0(),
		Pos:          it.AxPos.str("b"),
		Delete:       it.Delete.on(false),
		MajorGrid:    it.MajorGridlines != nil,
		MinorGrid:    it.MinorGridlines != nil,
		Title:        cr.title(it.Title),
		CrossAx:      it.CrossAx.int(0),
		Crosses:      it.Crosses.str("autoZero"),
		CrossBetween: it.CrossBetween.str(""),
		TickLblPos:   it.TickLblPos.str("nextTo"),
		MajorTick:    it.MajorTickMark.str("out"),
		MinorTick:    it.MinorTickMark.str("none"),
		LblSkip:      it.TickLblSkip.int(0),
		TickSkip:     it.TickMarkSkip.int(0),
		NoMultiLvl:   it.NoMultiLvlLbl.on(false),
		Line:         cr.shape(it.SpPr),
		Font:         cr.text(it.TxPr),
	}
	switch it.XMLName.Local {
	case "valAx":
		a.Kind = model.AxisVal
	case "dateAx":
		a.Kind = model.AxisDate
	case "serAx":
		a.Kind = model.AxisSer
	}
	if it.MajorGridlines != nil {
		a.MajorGridLine = cr.shape(it.MajorGridlines.SpPr)
	}
	if it.MinorGridlines != nil {
		a.MinorGridLine = cr.shape(it.MinorGridlines.SpPr)
	}
	if v, ok := it.CrossesAt.float(); ok {
		a.CrossesAt, a.HasCrossesAt = v, true
	}
	if s := it.Scaling; s != nil {
		a.Reversed = s.Orientation.str("minMax") == "maxMin"
		a.Min, a.HasMin = s.Min.float()
		a.Max, a.HasMax = s.Max.float()
		a.LogBase, _ = s.LogBase.float()
	}
	a.MajorUnit, _ = it.MajorUnit.float()
	a.MinorUnit, _ = it.MinorUnit.float()
	if it.NumFmt != nil {
		a.NumFmt = it.NumFmt.Code
		a.SourceLinked = it.NumFmt.SourceLinked == "1" || it.NumFmt.SourceLinked == "true"
	}
	return a
}

// AxID0 is an axis' own id (its first c:axId).
func (it *xPlotItem) AxID0() int {
	if len(it.AxID) == 0 {
		return 0
	}
	return it.AxID[0].int(0)
}

// --- references in document order (for saving the part as read) ---

// refSpan is a c:f element of a chart part: where its text is, where the
// cache that follows it is (cacheEnd 0 when none), and what it is.
type refSpan struct {
	at                   model.RawRef
	start, end           int // the f element's text
	cacheStart, cacheEnd int // the numCache/strCache element after it
	cacheNum             bool
}

// chartRefSpans walks a chart part's XML and lists its c:f elements in
// order, telling which series and field (or title) each belongs to.
func chartRefSpans(data []byte) ([]refSpan, bool) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var (
		stack           []string
		plot, axis, ser = -1, -1, -1
		out             []refSpan
		cur             = -1 // the c:f whose numRef/strRef we are in
		inF             bool
	)
	isChartNS := func(n xml.Name) bool { return n.Space == nsChart || n.Space == nsChartStrict }
	for {
		before := int(d.InputOffset())
		tok, err := d.Token()
		if err == io.EOF {
			return out, true
		}
		if err != nil {
			return nil, false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := t.Name.Local
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if isChartNS(t.Name) {
				switch {
				case parent == "plotArea" && strings.HasSuffix(n, "Chart"):
					plot++
					ser = -1
				case parent == "plotArea" && strings.HasSuffix(n, "Ax"):
					axis++
				case n == "ser" && strings.HasSuffix(parent, "Chart"):
					ser++
				case n == "f":
					out = append(out, refSpan{at: refContext(stack, plot, ser, axis), start: int(d.InputOffset())})
					cur, inF = len(out)-1, true
				case (n == "numCache" || n == "strCache") && cur >= 0 && !inF && (parent == "numRef" || parent == "strRef"):
					out[cur].cacheStart, out[cur].cacheNum = before, n == "numCache"
				}
			}
			stack = append(stack, n)
		case xml.CharData:
			if inF {
				out[cur].at.Text += string(t)
			}
		case xml.EndElement:
			n := t.Name.Local
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if isChartNS(t.Name) {
				switch {
				case n == "f" && inF:
					out[cur].end, inF = before, false
				case (n == "numCache" || n == "strCache") && cur >= 0 && out[cur].cacheStart > 0 && out[cur].cacheEnd == 0:
					out[cur].cacheEnd = int(d.InputOffset())
				case n == "numRef" || n == "strRef" || n == "multiLvlStrRef":
					cur = -1
				}
			}
		}
	}
}

// refContext tells what a c:f at the top of stack belongs to.
func refContext(stack []string, plot, ser, axis int) model.RawRef {
	if ser >= 0 && slicesContains(stack, "ser") {
		for i := len(stack) - 1; i > 0; i-- {
			if stack[i-1] != "ser" {
				continue
			}
			switch stack[i] {
			case "tx":
				return model.RawRef{Plot: plot, Series: ser, Field: model.RefName}
			case "cat", "xVal":
				return model.RawRef{Plot: plot, Series: ser, Field: model.RefCat}
			case "val", "yVal":
				return model.RawRef{Plot: plot, Series: ser, Field: model.RefVal}
			}
			break
		}
		return model.RawRef{Plot: -3}
	}
	if len(stack) > 0 && stack[len(stack)-1] == "strRef" && slicesContains(stack, "title") {
		if slicesContains(stack, "plotArea") {
			return model.RawRef{Plot: -2, Series: axis}
		}
		return model.RawRef{Plot: -1}
	}
	return model.RawRef{Plot: -3}
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// --- drawings ---

type xAnchorMarker struct {
	Col    int   `xml:"col"`
	ColOff int64 `xml:"colOff"`
	Row    int   `xml:"row"`
	RowOff int64 `xml:"rowOff"`
}

type xGraphicFrame struct {
	NvPr struct {
		CNvPr struct {
			Name string `xml:"name,attr"`
		} `xml:"cNvPr"`
	} `xml:"nvGraphicFramePr"`
	Graphic struct {
		Data struct {
			URI   string `xml:"uri,attr"`
			Chart *struct {
				Attrs []xml.Attr `xml:",any,attr"`
			} `xml:"chart"`
		} `xml:"graphicData"`
	} `xml:"graphic"`
}

type xAnchor struct {
	XMLName xml.Name
	EditAs  string         `xml:"editAs,attr"`
	From    *xAnchorMarker `xml:"from"`
	To      *xAnchorMarker `xml:"to"`
	Pos     *struct {
		X int64 `xml:"x,attr"`
		Y int64 `xml:"y,attr"`
	} `xml:"pos"`
	Ext *struct {
		CX int64 `xml:"cx,attr"`
		CY int64 `xml:"cy,attr"`
	} `xml:"ext"`
	Frame *xGraphicFrame `xml:"graphicFrame"`
	// Anything else drawn: pictures, shapes, groups, connectors.
	Pic   *struct{} `xml:"pic"`
	Sp    *struct{} `xml:"sp"`
	Group *struct{} `xml:"grpSp"`
	Cxn   *struct{} `xml:"cxnSp"`
	// Excel 2016's new chart types come wrapped in mc:AlternateContent.
	Alt []struct {
		Choice struct {
			Requires string `xml:"Requires,attr"`
		} `xml:"Choice"`
	} `xml:"AlternateContent"`
}

// readDrawing reads the charts of a drawing part; others counts the
// objects that aren't charts (pictures, shapes), newer charts those of
// Excel 2016's chart types.
func (r *reader) readDrawing(name string) (charts []*model.Chart, others, newer int) {
	var dr struct {
		Items []xAnchor `xml:",any"`
	}
	if err := r.unmarshal(name, &dr); err != nil {
		return nil, 0, 0
	}
	rels, _ := r.rels(name)
	byID := map[string]xRel{}
	for _, rel := range rels {
		byID[rel.ID] = rel
	}
	for _, it := range dr.Items {
		switch it.XMLName.Local {
		case "twoCellAnchor", "oneCellAnchor", "absoluteAnchor":
		case "AlternateContent":
			newer++
			continue
		default:
			continue
		}
		if len(it.Alt) > 0 {
			newer++
			continue
		}
		f := it.Frame
		if f == nil || f.Graphic.Data.Chart == nil {
			if f != nil || it.Pic != nil || it.Sp != nil || it.Group != nil || it.Cxn != nil {
				others++
			}
			continue
		}
		rid := ""
		for _, a := range f.Graphic.Data.Chart.Attrs {
			if a.Name.Local == "id" {
				rid = a.Value
			}
		}
		rel, ok := byID[rid]
		if !ok || !strings.HasSuffix(rel.Type, "/chart") {
			others++
			continue
		}
		c := r.readChart(rel.Target)
		if c == nil {
			others++
			continue
		}
		c.Name = f.NvPr.CNvPr.Name
		c.Anchor = anchorOf(&it)
		charts = append(charts, c)
	}
	return charts, others, newer
}

func anchorOf(it *xAnchor) model.Anchor {
	var a model.Anchor
	marker := func(m *xAnchorMarker) model.CellOffset {
		if m == nil {
			return model.CellOffset{}
		}
		return model.CellOffset{Row: max(m.Row, 0), Col: max(m.Col, 0), DX: max(m.ColOff, 0), DY: max(m.RowOff, 0)}
	}
	if it.Ext != nil {
		a.ExtX, a.ExtY = it.Ext.CX, it.Ext.CY
	}
	switch it.XMLName.Local {
	case "twoCellAnchor":
		a.Kind, a.EditAs = model.AnchorTwoCell, it.EditAs
		a.From, a.To = marker(it.From), marker(it.To)
	case "oneCellAnchor":
		a.Kind = model.AnchorOneCell
		a.From = marker(it.From)
	default:
		a.Kind = model.AnchorAbsolute
		if it.Pos != nil {
			a.PosX, a.PosY = it.Pos.X, it.Pos.Y
		}
	}
	return a
}

// readChart reads a chart part and keeps it (and the parts it uses) as
// read.
func (r *reader) readChart(name string) *model.Chart {
	rc, err := r.open(name)
	if err != nil {
		return nil
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil
	}
	c, err := parseChart(data, r.wb.Theme)
	if err != nil {
		return nil
	}
	raw := &model.ChartRaw{XML: data, Parts: map[string]*model.RawPart{}}
	if spans, ok := chartRefSpans(data); ok {
		for _, s := range spans {
			raw.Refs = append(raw.Refs, s.at)
		}
	} else {
		raw = nil
	}
	if raw != nil {
		raw.Rels = r.rawRels(name)
		r.keepParts(raw, raw.Rels, 0)
		c.Raw = raw
	}
	return c
}

// rawRels reads a part's relationships for keeping.
func (r *reader) rawRels(name string) []model.RawRel {
	rels, err := r.rels(name)
	if err != nil {
		return nil
	}
	out := make([]model.RawRel, 0, len(rels))
	for _, rel := range rels {
		out = append(out, model.RawRel{ID: rel.ID, Type: rel.Type, Target: rel.Target, External: rel.Mode == "External"})
	}
	return out
}

// keepParts reads the internal targets of rels (and theirs, a few levels
// down) into raw.Parts.
func (r *reader) keepParts(raw *model.ChartRaw, rels []model.RawRel, depth int) {
	if depth > 3 {
		return
	}
	for _, rel := range rels {
		if rel.External {
			continue
		}
		key := normPath(rel.Target)
		if _, done := raw.Parts[key]; done {
			continue
		}
		rc, err := r.open(rel.Target)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		p := &model.RawPart{ContentType: r.contentType(rel.Target), Data: data, Rels: r.rawRels(rel.Target)}
		raw.Parts[key] = p
		r.keepParts(raw, p.Rels, depth+1)
	}
}

// contentType returns a part's content type from [Content_Types].xml.
func (r *reader) contentType(name string) string {
	if r.types == nil {
		r.types = map[string]string{}
		var ct struct {
			Defaults []struct {
				Ext  string `xml:"Extension,attr"`
				Type string `xml:"ContentType,attr"`
			} `xml:"Default"`
			Overrides []struct {
				Part string `xml:"PartName,attr"`
				Type string `xml:"ContentType,attr"`
			} `xml:"Override"`
		}
		if r.unmarshal("[Content_Types].xml", &ct) == nil {
			for _, d := range ct.Defaults {
				r.types["."+strings.ToLower(d.Ext)] = d.Type
			}
			for _, o := range ct.Overrides {
				r.types[normPath(o.Part)] = o.Type
			}
		}
	}
	if t, ok := r.types[normPath(name)]; ok {
		return t
	}
	return r.types[strings.ToLower(path.Ext(name))]
}

// readSheetCharts reads the charts on a worksheet's drawing.
func (r *reader) readSheetCharts(sheetPart string, sh *model.Sheet) {
	rels, err := r.rels(sheetPart)
	if err != nil {
		return
	}
	others, newer := 0, 0
	for _, rel := range rels {
		if rel.Type != relDrawing && !strings.HasSuffix(rel.Type, "/drawing") {
			continue
		}
		cs, o, n := r.readDrawing(rel.Target)
		sh.Charts = append(sh.Charts, cs...)
		others += o
		newer += n
	}
	if others > 0 {
		r.wb.Warnings = append(r.wb.Warnings, sh.Name+": pictures and shapes aren't shown and are dropped when saved")
	}
	if newer > 0 {
		r.wb.Warnings = append(r.wb.Warnings, sh.Name+": charts of Excel 2016's newer types (waterfall, histogram, ...) aren't shown and are dropped when saved")
	}
}

// readChartSheet reads a chart sheet's chart.
func (r *reader) readChartSheet(part string, sh *model.Sheet) bool {
	rels, err := r.rels(part)
	if err != nil {
		return false
	}
	for _, rel := range rels {
		if !strings.HasSuffix(rel.Type, "/drawing") {
			continue
		}
		cs, _, _ := r.readDrawing(rel.Target)
		if len(cs) > 0 {
			sh.ChartSheet = cs[0]
			return true
		}
	}
	return false
}
