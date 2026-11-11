# Keyboard and mouse reference: Windows and Linux

Every key binding in ExcelLite and WordLite on Windows and Linux, and in
the web version on those systems, taken from the code (`excel/ui`,
`word/ui`, `common/gui`). The feature guides ([EXCEL.md](./EXCEL.md),
[WORD.md](./WORD.md)) mention shortcuts as they describe features; this
page lists them all in one place. For macOS see
[KEYBOARD-MAC.md](./KEYBOARD-MAC.md).

Shortcuts marked *grid* or *page* only work while the sheet grid or the
document page has the keyboard focus. They do nothing while a text box,
the Find bar or a pane has the focus. The others work anywhere in the
window, unless a dialog is open or the File page is showing.

## Both applications

### Window and files

| Keys | Action |
| --- | --- |
| Ctrl+N | New window (a blank workbook or document) |
| Ctrl+O | File page, Open |
| Ctrl+F12 | Open file dialog |
| Ctrl+S | Save |
| F12 | File page, Save As |
| Ctrl+P | File page, Print (export to PDF) |
| Ctrl+W | Close the window (asks about unsaved changes) |
| Esc | Close the topmost menu, drop-down, dialog or the File page; then cancel whatever is in progress (see each app) |

### Menus, dialogs and panes

- The menu bar, ribbon drop-downs, color picker, Undo/Redo history lists
  and right-click menus are mouse-only. Esc closes an open menu-bar menu
  and right-click menu. There are no Alt access keys, F10 or Shift+F10.
- In dialogs, Tab / Shift+Tab move between controls, and Enter or Space
  presses the focused button. Enter in a dialog's text box usually means OK,
  and Esc means Cancel. Exceptions are listed under each app.
- Message boxes (for example "Save changes?") take Esc for Cancel; pick
  the other answers with the mouse.
- Text boxes (dialogs, Find, the formula bar) have the usual editing keys:
  arrows, Home/End, Shift to select, Ctrl+A/C/X/V/Z, Ctrl+Backspace.
- The Open and Save As dialogs are the system's own, with its keys.

### Scrolling

| Mouse | Action |
| --- | --- |
| Wheel | Scroll |
| Shift+wheel | Scroll sideways |
| Ctrl+wheel | Zoom in or out, 10% per step |
| Shift+click in a scroll bar's track | Jump the thumb to the pointer |
| Hold a scroll bar arrow or the track | Keep scrolling |

## ExcelLite

### Workbook

| Keys | Action |
| --- | --- |
| Ctrl+Alt+= or Ctrl+Alt++ | Zoom in 10% |
| Ctrl+Alt+- | Zoom out 10% |
| Ctrl+0 | Zoom to 100% |
| F9 | Recalculate all formulas |
| Shift+F11 | Insert a worksheet |
| F11 | Insert a chart sheet |
| Alt+F1 | Insert a clustered column chart of the data |
| Ctrl+PageDown / Ctrl+PageUp | Next / previous sheet (wraps around) |

Esc works through this list and stops at the first item that applies:
close a menu, filter or validation drop-down, or dialog; leave the File
page; abandon a sheet-tab rename or chart-title edit; cancel the cell
entry; stop the Format Painter; select the whole chart instead of a chart
part; clear the copy marquee and the chart selection.

### Moving and selecting (grid)

| Keys | Action |
| --- | --- |
| Arrows | Move one cell (hidden rows and columns are skipped) |
| Shift+arrows | Extend the selection |
| Ctrl+arrows | Jump to the edge of the data block |
| Ctrl+Shift+arrows | Extend the selection to the edge of the data block |
| Tab / Shift+Tab | Move right / left |
| Enter / Shift+Enter | Move down / up |
| PageDown / PageUp | Move one screen down / up (Shift extends) |
| Home | First column of the row |
| Ctrl+Home | Top-left cell (below and right of frozen panes) |
| Ctrl+End | Last cell of the used range |
| Shift with Home, Ctrl+Home or Ctrl+End | Extend the selection there |
| Ctrl+A | Select the whole sheet |

Enter and Tab collapse a multi-cell selection; they don't step through it
as in Excel. End on its own does nothing (there is no End mode).

### Entering and editing cells

| Keys | Action |
| --- | --- |
| Typing | Replace the cell's content (Enter mode) |
| F2 | Edit the cell's content (Edit mode); while editing, switch between Enter and Edit mode |
| Backspace (grid) | Clear the cell and start an empty entry |
| Delete (grid) | Clear contents (formats stay) |
| Enter / Shift+Enter | Commit and move down / up |
| Tab / Shift+Tab | Commit and move right / left |
| Ctrl+Enter | Commit and stay |
| Alt+Enter | Line break in the cell |
| Arrows | Enter mode: commit and move. Edit mode and in the formula bar: move the caret |
| Esc | Cancel the entry |
| Shift+F3 | Insert Function (also works while editing) |

While a cell is being edited, the grid, formatting and Insert shortcuts
are off, except Shift+F3.

### Formatting (grid)

| Keys | Action |
| --- | --- |
| Ctrl+B / Ctrl+I / Ctrl+U | Bold / italic / underline |
| Ctrl+5 | Strikethrough |
| Ctrl+1 | Format Cells |
| Ctrl+Shift+~ | General format |
| Ctrl+Shift+$ | Currency (`$#,##0.00`) |
| Ctrl+Shift+% | Percent |
| Ctrl+Shift+# | Date (`d-mmm-yy`) |
| Ctrl+Shift+@ | Time (`h:mm AM/PM`) |
| Ctrl+Shift+! | Number (`#,##0.00`) |
| Esc | Stop the Format Painter |

### Clipboard, undo and structure (grid)

| Keys | Action |
| --- | --- |
| Ctrl+C / Ctrl+X / Ctrl+V | Copy / cut / paste |
| Ctrl+Z | Undo |
| Ctrl+Y | Redo (Ctrl+Shift+Z isn't bound) |
| Ctrl+D / Ctrl+R | Fill down / right |
| Ctrl++ or Ctrl+Shift+= | Insert cells, rows or columns |
| Ctrl+- | Delete cells, rows or columns |
| Shift+Alt+Right / Shift+Alt+Left | Group / ungroup (outline) |

### Find, Go To and Insert

| Keys | Action |
| --- | --- |
| Ctrl+F | Find |
| Ctrl+H | Replace |
| Ctrl+G or F5 | Go To |
| Ctrl+K | Insert hyperlink |
| Shift+F2 | New or edit note |
| Shift+F3 | Insert Function |

- Find & Replace: Enter in either box is Find Next; Esc closes. The dialog
  isn't modal, so you can keep working in the sheet.
- Go To: Enter goes to the reference; double-clicking a name goes there.
- Insert Function and Symbol: double-click an entry to insert it.
- Hyperlink: Enter in any box is OK.
- Note: Enter types a line break, so use OK to save. Esc discards the
  note.

### Data

| Keys | Action |
| --- | --- |
| Ctrl+Shift+L | AutoFilter on / off |
| Alt+Down | On a filter header: open its filter drop-down. Elsewhere: open the cell's validation list |

- Filter and PivotTable field drop-downs: Enter in the search box applies
  the checklist like OK; Esc closes.
- Validation list: Up / Down (or Alt+Down) move, Enter or Tab picks, Esc
  closes.

### Charts

These keys work while a chart is selected.

| Keys | Action |
| --- | --- |
| Arrows | Nudge the chart 1 pixel |
| Ctrl+arrows | Nudge the chart a quarter pixel |
| Delete / Backspace | Delete the selected part (title, legend, axis, series...) or the chart |
| F2 or Enter, title selected | Edit the title (Enter commits, Esc abandons) |
| Typing, title selected | Replace the title |
| Ctrl+C / Ctrl+X / Ctrl+V | Copy / cut / paste the chart |
| Ctrl+Z / Ctrl+Y | Undo / redo |
| Esc | Chart part selected: select the whole chart; then deselect it |

Other grid keys (Tab, Home, Ctrl+A and so on) deselect the chart and act
on the cells. On a chart sheet only undo, redo, copy and sheet switching
work.

### Dialogs

Format Cells, Insert Function, Hyperlink, Go To, Custom Filter, Top 10,
Text to Columns, Data Validation, the chart dialogs and the PivotTable
dialogs: Enter is OK and Esc is Cancel. In the sheet-tab rename box, Enter
or clicking away commits and Esc abandons.

### Mouse

| Mouse | Action |
| --- | --- |
| Click | Select a cell, a column or row (header), or the sheet (corner) |
| Shift+click | Extend the selection |
| Ctrl+click or Ctrl+drag | Add another range to the selection |
| Double-click a cell | Edit it (on a PivotTable value: Show Details) |
| Ctrl+click a hyperlink | Follow it |
| Drag the selection border | Move the cells; hold Ctrl when releasing to copy |
| Drag the fill handle | Fill a series or copy |
| Drag a header divider | Resize the column or row |
| Double-click a header divider | AutoFit the column or row |
| Click or drag while typing a formula | Insert a reference (Shift+click extends it) |
| Hover a cell with a note | Show the note |
| Right-click | Context menu (chart menu over a chart) |
| Click the formula bar | Edit the cell there |
| Double-click a sheet tab | Rename it |
| Drag a chart / its handles | Move / resize it; Alt snaps to cell edges, Shift keeps a corner's proportions |
| Double-click a chart title | Edit it |
| Format Painter: click / double-click | Paint once / keep painting until Esc |

## WordLite

### Window and view

| Keys | Action |
| --- | --- |
| Ctrl++ or Ctrl+= | Zoom in 10% |
| Ctrl+- | Zoom out 10% |
| Ctrl+0 | Zoom to 100% |

Esc closes the topmost dialog, menu or the File page. If none is open, it
cancels a text move or table-border drag, closes the right-click menu,
turns off the Format Painter, closes the Find bar, or leaves the header or
footer. The focus then returns to the page.

### Moving and selecting (page)

Add Shift to any of these to extend the selection.

| Keys | Action |
| --- | --- |
| Left / Right | One character (a plain arrow collapses a selection to that side) |
| Ctrl+Left / Ctrl+Right | One word |
| Up / Down | One line |
| Ctrl+Up / Ctrl+Down | Previous / next paragraph |
| Home / End | Start / end of the line |
| Ctrl+Home / Ctrl+End | Start / end of the document |
| PageUp / PageDown | One screen |
| Ctrl+A | Select all |

### Editing (page)

| Keys | Action |
| --- | --- |
| Backspace / Delete | Delete back / forward (or the selection) |
| Ctrl+Backspace / Ctrl+Delete | Delete the previous / next word |
| Enter | New paragraph |
| Shift+Enter | Line break |
| Ctrl+Enter | Page break |
| Tab | Table: next cell. Start of a list item: demote. Otherwise: a tab |
| Shift+Tab | Table: previous cell. Start of a list item: promote |
| Ctrl+Tab | A tab character, even in a table |
| Ctrl+C / Ctrl+X / Ctrl+V | Copy / cut / paste |
| Ctrl+Alt+V | Paste Special |
| Ctrl+Z | Undo |
| Ctrl+Y or Ctrl+Shift+Z | Redo |

In a table, Tab in the last cell adds a row.

### Formatting

| Keys | Action |
| --- | --- |
| Ctrl+B / Ctrl+I / Ctrl+U | Bold / italic / underline (page) |
| Ctrl+L / Ctrl+E / Ctrl+R / Ctrl+J | Align left / center / right / justify (page) |
| Ctrl+Space | Clear character formatting (page) |
| Ctrl+Shift+N | Normal style (page) |
| Ctrl+Alt+1 / 2 / 3 | Heading 1 / 2 / 3 (page) |
| Ctrl+Shift+> / Ctrl+Shift+< | Grow / shrink the font |
| Ctrl+Shift+C / Ctrl+Shift+V | Copy / paste formatting |
| Ctrl+Shift+8 | Show/Hide ¶ |
| Ctrl+Alt+Shift+S | Styles pane on / off |

### Find, fields and insert

| Keys | Action |
| --- | --- |
| Ctrl+F | Find bar |
| Ctrl+H | Replace |
| Ctrl+G | Go to page |
| F3 / Shift+F3 | Next / previous match (F3 with an empty Find box opens Find) |
| F9 | Update the fields in the selection (and the table of contents) |
| Alt+F9 | Show / hide field codes |
| Ctrl+F9 | Field dialog |
| Ctrl+K | Insert or edit a link (page) |
| Ctrl+Alt+M | New comment (page) |

- Find bar: Enter in Find is the next match and Shift+Enter the previous
  one; Enter in Replace replaces one; Enter in the page box goes to the
  page. Esc closes the bar.
- Table, Link, Field, Page Number Format and New Style dialogs: Enter in a
  text box is OK.
- Date and Time: Up / Down choose a format, Enter (or a double-click)
  inserts it.
- Comments pane: Enter types a line break, so use the button to post.

### Mouse

| Mouse | Action |
| --- | --- |
| Click | Place the caret |
| Shift+click | Extend the selection |
| Double-click / triple-click | Select the word / paragraph; dragging afterward extends by words / paragraphs |
| Drag past the top or bottom | Auto-scroll |
| Left margin: click | Select the line (beside a table: the row) |
| Left margin: double-click | Select the paragraph |
| Left margin: triple-click or Ctrl+click | Select the document |
| Left margin: Shift+click or drag | Extend by lines |
| Drag the selection | Move it; hold Ctrl when releasing to copy; Esc cancels |
| Ctrl+click a link | Open it |
| Click a picture / drag its handles | Select / resize it |
| Drag a table border | Resize the column or row (Esc cancels) |
| Double-click the header or footer | Edit it; double-click the body to leave |
| Right-click | Context menu |
| Format Painter: click / double-click | Paint once / keep painting until Esc |
| Styles pane: click / double-click | Select / apply a style |

## Differences from Excel and Word

Keys that do something different from Microsoft Office, or aren't bound:

- **Zoom.** ExcelLite zooms with Ctrl+Alt+= / Ctrl+Alt+- and WordLite with
  Ctrl+= / Ctrl+-. Ctrl+0 resets the zoom in both. In Office, Ctrl+0 hides
  columns, and Ctrl+= / Ctrl+Shift+= are subscript / superscript in Word.
  WordLite has no subscript or superscript keys (use the ribbon).
- **ExcelLite.** Ctrl+A selects the whole sheet at once. Enter and Tab
  don't step through a selection. Ctrl+Enter in the grid moves down
  instead of filling the selection. Ctrl+PageDown wraps to the first sheet.
  End mode, F4, Ctrl+; (date), Ctrl+Shift+; (time), Ctrl+Space,
  Shift+Space, Ctrl+` (show formulas), Alt+= (AutoSum), Ctrl+T, Ctrl+9,
  Ctrl+Shift+6, Ctrl+Shift+7 and Ctrl+Shift+Z are not bound.
- **WordLite.** Shift+F3 is find previous (Word: Change Case).
  Ctrl+Shift+Enter is a page break (Word: column break). Shift+F9 isn't
  bound. Ctrl+PageUp/PageDown act like PageUp/PageDown.
- **Menus.** There are no Alt access keys, KeyTips, F10 or Shift+F10, so
  the menus and ribbon need the mouse.
