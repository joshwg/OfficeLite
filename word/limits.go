// Limits of WordLite, all in one place: the document model's own caps
// (table size, font size, the ranges the number formats spell out), the
// .doc, .docx and Pages readers' and writers' bounds, the layout engine's
// shaping and picture-cache sizes, and the editor window's timings.
// Each group's prefix names the package that uses it (Doc for docfile,
// Docx for docxfile, Pages for pagesfile, Layout, UI); the model's own
// have none. This package imports no other word/ package, so all of them
// can use these.

package word

import "time"

// Table size.
const (
	// MaxTableSize is the largest number of rows or columns Insert Table
	// creates (Word allows 63 columns).
	MaxTableSize = 63
	// MaxTableRows is the most rows Insert Table creates: 32,767, Word's
	// limit for the Insert Table dialog.
	MaxTableRows = 32767
)

// MaxFontSize is the largest font size, in points, Grow Font steps to:
// 1,638 pt, Word's largest font size.
const MaxFontSize = 1638

// UndoHistory is how many changes Undo can take back (as in ExcelLite);
// the oldest steps past it are dropped.
const UndoHistory = 100

// MaxComments bounds how many comments are read from a .doc file.
const MaxComments = 100_000

// Number formats.
const (
	// MaxAlpha is the largest number Alpha spells in letters: z thirty
	// times, Word's limit.
	MaxAlpha = 780
	// MaxRoman is the largest number Roman writes in roman numerals: 3,999
	// (MMMCMXCIX), the largest with no symbol above M.
	MaxRoman = 3999
	// MaxSpelled bounds CardText and OrdText: a number of a million or
	// more is written in digits, as Word's \* CardText does.
	MaxSpelled = 1_000_000
)

// UndoLabelRunes is how many characters of typed text the Undo list's
// "Typing '…'" label shows before cutting it off with an ellipsis.
const UndoLabelRunes = 20

// .doc reader bounds (word/fileio/docfile): a damaged or hostile file
// can't exhaust the stack or memory.
const (
	// DocMaxTableDepth bounds table nesting: deeper paragraphs go in the
	// innermost table allowed (Word itself nests tables 63 deep at most).
	DocMaxTableDepth = 64
	// DocMaxShapeDepth bounds how deeply collectShapes follows nested
	// shape groups, so a hostile file can't exhaust the stack.
	DocMaxShapeDepth = 100
	// DocMaxSectColumns is the most text columns of a section whose widths
	// and spacings are read (sprmSDxaColWidth/ColSpacing index a column in
	// one byte): a larger index is ignored rather than growing the lists.
	DocMaxSectColumns = 64
)

// .doc writer measurements, in twips, it clamps to.
const (
	// DocMaxTw is the largest measurement written, in twips: 31,680 (22
	// inches), Word's largest page size, indent, spacing or tab position.
	DocMaxTw = 31680
	// DocMinPageTw is the smallest page width or height written, in
	// twips: 144 (0.1 inch), Word's smallest page size.
	DocMinPageTw = 144
)

// .doc writer font sizes, in half-points, it clamps to.
const (
	// DocMinHps is the smallest font size written: 2 half-points (1 pt),
	// Word's smallest.
	DocMinHps = 2
	// DocMaxHps is the largest font size written: 3,276 half-points
	// (1,638 pt, MaxFontSize), Word's largest.
	DocMaxHps = 3276
)

// .doc writer record sizes and counts it cuts to.
const (
	// DocMaxChpxGrpprl is the longest character-property grpprl a CHPX
	// holds: a CHPX's length is one byte, so formatting beyond 255 bytes
	// is left out.
	DocMaxChpxGrpprl = 255
	// DocMaxFkpPapx is the most bytes (2 plus the grpprl) a paragraph's
	// properties may take in a PAPX FKP; a larger grpprl goes in the Data
	// stream through sprmPHugePapx.
	DocMaxFkpPapx = 400
	// DocMaxRowCells is the most cells a table row holds in a .doc
	// (Word's 63-column limit, as MaxTableSize); longer rows are cut.
	DocMaxRowCells = 63
	// DocMaxTabChanges is the longest delete or add list encTabs writes
	// in a sprmPChgTabsPapx operand (each list's count is one byte);
	// further tab stops are dropped.
	DocMaxTabChanges = 50
	// DocMaxFontName is the longest font name written to the font table,
	// in UTF-16 code units: 31, so the name and its NUL fit Windows'
	// 32-character LF_FACESIZE.
	DocMaxFontName = 31
	// DocMaxStyleName is the longest style name written to the style
	// sheet, in UTF-16 code units; a longer name is cut.
	DocMaxStyleName = 253
)

// .docx reader bounds (word/fileio/docxfile): a damaged or hostile file
// can't exhaust the stack or memory.
const (
	// DocxMaxXMLDepth bounds element nesting: the readers walk the tree
	// recursively, and no real document comes near it.
	DocxMaxXMLDepth = 2000
	// DocxMaxPartSize is the largest part of the zip package read: 1 GiB
	// (2³⁰), well past any real part, so a zip bomb can't take all of
	// memory; a larger part fails the open as damaged or too large.
	DocxMaxPartSize = 1 << 30
)

// PagesMaxTableRows bounds the rows of a table the Pages importer
// (word/fileio/pagesfile) brings into a document; the rest are dropped
// with a warning.
const PagesMaxTableRows = 5000

// Layout engine (word/layout).
const (
	// LayoutMaxShapePx is the largest size, in pixels per em, text is
	// shaped at: Gio keeps the size in 16 bits of 26.6 fixed point in its
	// glyph IDs (and panics past 1023 px), so larger text is shaped at
	// this size and scaled.
	LayoutMaxShapePx = 1000
	// LayoutUnwrappedWidth is the MaxWidth, in pixels, a single run of
	// text is shaped with: 2²⁴, far wider than any line, so the shaper
	// never wraps it (line breaking is the layout engine's job).
	LayoutUnwrappedWidth = 1 << 24
	// LayoutMinPicBucket is the smallest width, pixels, of the scaled copy
	// a picture is drawn from (the next power of two at or above its drawn
	// width, within LayoutMinPicBucket..LayoutMaxPicBucket).
	LayoutMinPicBucket = 16
	// LayoutMaxPicBucket is the largest width, pixels, of a picture's
	// scaled copy: 8,192, so a huge picture at a high zoom doesn't fill
	// the GPU's memory.
	LayoutMaxPicBucket = 8192
)

// Editor window timings (word/ui).
const (
	// UIBlinkInterval is how long the caret stays on, then off, as it
	// blinks: 530 ms, Windows' default caret blink time.
	UIBlinkInterval = 530 * time.Millisecond
	// UIDoubleClickTime is the most time between two clicks at the same
	// spot for them to count as a double (or triple) click: 500 ms,
	// Windows' default double-click time.
	UIDoubleClickTime = 500 * time.Millisecond
)

// Editor window caps (word/ui).
const (
	// UIMaxScroll bounds the scroll distance, in pixels, one pointer event
	// may report to the page view: 2²⁰, more than any real wheel or drag.
	UIMaxScroll = 1 << 20
	// UIMaxFindSeed is the longest selection, in characters, that Find
	// and Replace starts its "Find what" box with (a longer one, or one
	// spanning paragraphs, leaves the box as it was): Word's 255.
	UIMaxFindSeed = 255
	// UIMaxInfoStyles is how many style names Backstage's Info page lists
	// before summing up the rest as "and n more".
	UIMaxInfoStyles = 12
	// UIMaxStartPage is the largest number the Format Page Numbers
	// dialog's "Start at" accepts: 32,767, Word's limit.
	UIMaxStartPage = 32767
)
