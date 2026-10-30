# Architecture and shared behavior

See [the documentation overview](./README.md) for build instructions and
links to the application guides.

OfficeLite is a single Go application. The launcher detects file content and
routes each document to ExcelLite or WordLite; windows share common UI,
font, PDF, recent-file and file-handling code. ExcelLite contains the
spreadsheet model, calculation, editing, rendering and file-format packages.
WordLite contains the document model, editing, pagination/layout, rendering
and file-format packages.

## Shared application interface

File opens a full-window page, as in Excel, with a side menu:

- **Home**: the New tiles (blank workbook, blank document) and the Recent list
- **New**: blank workbook or document, each in a new window
- **Open**: Browse… plus the full Recent list; a file opens in this window if
  it is still an untouched new workbook, otherwise in a new one (documents go
  to a WordLite window)
- **Info**: file path, type, size, modified time, sheets, used range, cell,
  formula and defined-name counts, date system, items not shown, version
- **Save**: writes the workbook back to its file in that file's format
  (a new workbook goes to Save As)
- **Save As**: Excel Workbook (`.xlsx`) or Excel 97-2003 Workbook (`.xls`),
  all sheets, or CSV or tab-delimited text (values of the active sheet); the
  saved file becomes the window's document. Before saving in a format that
  can't hold everything, a prompt lists what would be lost (once per file)
- **Print**: Print via PDF — the sheet (or workbook, or document) is written
  to a temporary PDF and opened in the default PDF viewer to print from
  there; direct printing isn't available yet
- **Export**: CSV or tab-delimited copy of the active sheet, or **PDF**: the
  active sheet or the entire workbook as Excel prints it (used range, Letter
  pages down then across, Excel's normal margins, column widths, row heights,
  formatting, borders, merged cells; gridlines and "Fit all columns on one
  page" optional), with the fonts embedded (subsetted) and the text
  searchable; the PDF opens in the default viewer afterwards (optional).
  WordLite exports its pages as laid out (tables, shading, pictures,
  headers/footers)
- **Close**: closes the window

The Recent list (up to 50 files, both apps) is kept in the user's config
directory, `%AppData%\OfficeLite\recent.json` on Windows.

**Drag and drop** (Windows): files dragged from Explorer onto any
OfficeLite window — the sheet, the page, the ribbon or the File page —
open as File > Open opens them: the first in this window if it is an
untouched new workbook or document, the rest in new windows, each in the
right app (a `.docx` dropped on ExcelLite opens in WordLite), and a file
already open brings its window to the front. Up to 20 files open per drop.
Anything else (programs, pictures, folders) is never run or inserted: the
status bar says "OfficeLite can't open …". A drop while a dialog is open
is ignored. It works when OfficeLite runs as administrator too.
## File formats

`.numbers` and `.pages` files (iWork 2013 and later: a zip of Snappy-compressed
protobuf archives, `Index/*.iwa`) can be opened by content from the Open
dialog, the command line or a double-click. They are imported, not edited in
place; Save goes to Save As in OfficeLite's formats. iWork '09 files (gzipped
XML) and password-protected documents are not supported. The imported content
is described in [EXCEL.md](./EXCEL.md#importing-apple-numbers-files) and
[WORD.md](./WORD.md#importing-apple-pages-documents).

## Files open elsewhere

Opening a file that is already open in an OfficeLite window brings that
window to the front instead, whether it was opened from File > Open, the
recent list or the command line, and also from a second `officelite.exe`
(an Explorer double-click): each open document is listed in
`officelite/open/` under the user's cache directory (pid, path, and a
localhost port and token of the process that has it), and the new process
hands the file over and exits. Entries of processes that are gone are
ignored and removed.

Before opening, a file is checked for Office's signs of use (common/lockfile):

- the hidden **owner file** Word and Excel keep next to an open document,
  `~$` plus the name (Word drops the first two characters of a name of
  eight or more characters before the extension, and the first of a
  seven-character one: `~$port2024.docx` for `Report2024.docx`). It counts
  only while live — on Windows, while it can't be opened exclusively;
  elsewhere, for a day. A stale one naming the current user (a crash) is
  deleted; other users' are left alone;
- a **sharing lock** on the document itself (another program has it open
  against writing, as Word and Excel do);
- another OfficeLite process that has it open but couldn't be reached.

The dialog says "… is locked for editing by Alice" (or "in use by another
program") and offers Open Read-Only, Open Anyway and Cancel. Read-only
windows show "(Read-Only)" in the title and Save goes to Save As. Saving
over a file another program holds open says so ("… is open in another
program (Word or Excel?) — save a copy with Save As") instead of a bare
error, and leaves no temporary file.

On a **network share** (a `\\server\share` path or a mapped drive) OfficeLite
writes an Office-compatible owner file itself for `.docx`/`.doc`/`.xlsx`/
`.xls` files, so that Word and Excel users see "locked for editing by
<your name>" (the Windows display name, else the login name). It is hidden,
held open while the document is, moved on Save As and deleted on close;
local folders and OneDrive/Dropbox/Google Drive/iCloud folders get none,
and a share that can't be written to is skipped silently. OfficeLite does
not keep the document itself open, so an Office user who ignores the
owner file can still save over it.

## AutoRecover

As in Office, every 5 minutes each window whose document has unsaved
changes writes a recovery copy to `%LOCALAPPDATA%\OfficeLite\Recovery`
(`~/.cache/OfficeLite/Recovery` on Linux), never next to the document:
always `.xlsx` / `.docx`, whatever the document's own format (`.xls`,
`.doc`, `.csv`, `.txt`, `.numbers`, `.pages`, read-only and locked files
included), with a small JSON sidecar (original path, name, app, original
format, time, pid). Only the model is cloned on the UI goroutine (a few
ms for 100,000 cells); the file is written in the background, atomically,
and not again until the document changes. Saving, closing the window
(Don't Save included) and exiting delete the copy; a Save or close never
waits for one being written.

The interval is set on File ▸ Info ▸ AutoRecover (Off, 1, 2, 5, 10, 15 or
30 minutes), kept in `settings.json` in the user's config directory
(`%APPDATA%\OfficeLite`) and shared by every window.

When OfficeLite starts and finds copies whose process is gone (a crash,
power loss, a kill from Task Manager — on Windows the process's start time
is checked too, so a reused pid doesn't hide them), a **Document Recovery**
window lists them (name, app, time, original location) with Open, Save As…
and Discard. Open opens the copy in ExcelLite or WordLite as "Budget.xls
[Recovered]", modified; Save goes to Save As, starting from the original
folder, name and format. Save As… writes it straight to a file; Close
keeps the rest for the next start. Copies of another running OfficeLite
are left alone, and copies nobody dealt with are deleted after 30 days.

## AutoSave

As in Microsoft 365, the title bar has an **AutoSave** switch left of the
file name. When it is on, the document is saved into its own file about
3 seconds after the last edit (and at least once a minute while the edits
keep coming), through the same atomic save as Ctrl+S (a temporary file
renamed over the original): the model is cloned on the UI goroutine and
written in the background, and the title reads "Saving…", then "Saved".
Turning it on saves at once; Ctrl+S still works (it waits for an AutoSave
being written, and an older one never lands over it); Undo still reaches
back past the saves, since it is in memory; and after a save the
AutoRecover copy is deleted and closing the window doesn't ask, unless
something changed since. A save that fails (the file open in Word or
Excel, a full disk, no permission, a folder that is gone) turns AutoSave
off for that window and says why in the status bar; the edits stay in the
window and in its AutoRecover copy.

The switch works only for a saved `.xlsx` or `.docx` file — the formats
that keep everything — and is greyed, with a tooltip saying why, for new
documents, `.xls`/`.doc`/`.csv`/`.txt` files, imported `.numbers`/`.pages`
files, `[Recovered]` and read-only windows and files opened with Open
Anyway while someone else has them. Unlike OneDrive there is **no version
history** behind it: what AutoSave writes replaces the file, and Undo is
the only way back. That is why it is off for every document unless you
turn it on; File ▸ Info ▸ AutoSave sets whether it starts on for the
`.xlsx`/`.docx` files you open or save (`autoSaveDefault` in
`settings.json`, default Off).

## Layout

```
cmd/officelite/    the app: routes each file to an Excel or Word window;
                   its icon (officelite.png/.ico) and the Windows resource
                   linked into the .exe (rsrc_windows_amd64.syso)
cmd/mkicon/        draws the icon (go generate ./cmd/officelite)
common/            shared by Excel and Word
  colors/            color parsing and mixing
  doctype/           spreadsheet vs. document detection
  gui/               Gio helpers: font library, painting, scrollbar, theme,
                     menu bar, ribbon and its controls (shared by both apps),
                     modal dialogs and form rows, File backstage (Home/New/
                     Open pages), PDF export/print plumbing, native
                     Open/Save dialogs, files dropped from Explorer
  pdf/               PDF 1.7 writer: embedded subsetted fonts, images (PDF
                     export)
  recent/            recent-files list shared by all windows
  version/           the software version
  ole/               OLE2 compound files (.xls, .doc containers)
  iwork/             Apple iWork (.numbers/.pages) archives: Snappy, protobuf
                     fields, object index, styles, text storages, tables
  textenc/           text decoding (BOMs, UTF-16, Windows-1252)
  opc/               OPC package part names (.docx/.xlsx relationships)
excel/
  model/             workbook/sheet/cell/style model, size limits, units;
                     charts and their live data (chart.go, chartdata.go)
  fileio/            Open/Read: picks the reader from the file content;
                     Write/SaveFile: .xlsx, .csv, tab-delimited text
    xlsxfile/          .xlsx (streamed sheet XML; excelize for styles,
                       notes and hyperlinks; charts: chart.go, chartwrite.go)
    xlsfile/           .xls BIFF8 reader and writer (incl. notes, links)
    csvfile/           .csv/.tsv (delimiter sniffing, Excel-style typing)
    numbersfile/       .numbers import (tables, formats, styles, formulas)
  numfmt/            Excel number formats (General, dates, currency, ...)
  formula/           formula engine: parser, values, evaluator, criteria, dates
    functions/         worksheet functions, one package per category
  calc/              workbook recalculation, spilling, circular references,
                     incremental recalculation after edits
  pivot/             PivotTable engine: source fields and items, grouping
                     and summaries, compact/outline/tabular layout,
                     GETPIVOTDATA lookup, drill-down records
  edit/              editing: typed input, clipboard and paste modes,
                     formatting, insert/delete rows, columns, cells and
                     sheets, fill, sort, AutoFilter, Text to Columns, Remove
                     Duplicates, data validation, outline and subtotals,
                     PivotTables, find/replace, notes and links, undo/redo
  render/            draws the grid: panes, fills, text, borders, selection,
                     AutoFilter buttons, outline gutters, charts (chart*.go:
                     layout, axis scaling, Gio and PDF painting); AutoFit
                     measuring; PDF export
                     (pdf.go)
  ui/                window: title/menu/formula bars, Home and Data ribbons,
                     dialogs (Format Cells, Insert, Find, Sort, Custom
                     AutoFilter, ...), AutoFilter drop-downs, tabs, status
                     bar, input, File pages (Info, Save As, Print, Export)
  cmd/excelshot/     headless PNG renderer (dev tool)
  cmd/calccheck/     recalculate and compare with Excel's saved values
  cmd/chartsample/   writes Samples/Charts.xlsx (internal/chartsample)
word/              WordLite
  model/             document model: blocks, runs, styles, lists; editing
                     operations and undo (edit.go); table editing,
                     pictures and links, find, Format Painter,
                     sections, headers/footers, notes, floating objects,
                     tables of contents, style management
  fileio/            Open/Read/Write/SaveFile, picks the reader by content
    docxfile/          .docx/.docm/.dotx (keeps unmodeled parts on re-save)
    docfile/           .doc Word 97-2003 binary (FIB, piece table, STSH,
                       sections, subdocuments, OfficeArt shapes)
    pagesfile/         .pages import (styles, lists, tables, text boxes)
  layout/            line breaking, tabs, lists, tables, pagination by
                     section and column, headers/footers and page fields,
                     notes, floating objects, pictures, drawing; PDF
                     export (pdf.go)
  ui/                window: Home ribbon, Insert menu and dialogs, page
                     view, caret/selection, find bar, header/footer
                     editing, Styles pane, File pages
  cmd/wordshot/      headless PNG renderer (dev tool)
Fonts/             local fonts, not committed (see Fonts/README.md)
Samples/           sample workbooks and documents for trying features by
                   hand (Go test fixtures live in each package's testdata/)
```

## Fonts

Fonts are indexed from `./Fonts`, `OFFICELITE_FONTS`, the OS font folders and
Office's cloud-font cache (where Aptos lives); only the families a workbook
uses are loaded. Embedded bitmap strikes (Calibri has them) are ignored so
text always renders from outlines.

On Windows, document and cell text is rasterized by DirectWrite (hinted,
grayscale antialiased, as Word draws it) from the glyphs and positions Gio's
shaper laid out, so layout is the same on every OS; elsewhere, and for text
DirectWrite can't take (right-to-left runs, the built-in Go fonts), Gio fills
the glyph outlines. `OFFICELITE_TEXT=gio` turns DirectWrite off; see
`common/gui/systext_windows.go` for the other tuning variables.
