# Keyboard and mouse reference: macOS

Every key binding in ExcelLite and WordLite on macOS, and in the web
version on a Mac, taken from the code (`excel/ui`, `word/ui`,
`common/gui`). The feature guides ([EXCEL.md](./EXCEL.md),
[WORD.md](./WORD.md)) mention shortcuts as they describe features; this
page lists them all in one place. For Windows and Linux see
[KEYBOARD-WINDOWS.md](./KEYBOARD-WINDOWS.md).

## Mac keys

- **Cmd** takes the place of Ctrl in every shortcut and Ctrl+click, and
  **Option** takes the place of Alt. The Control key isn't used by the
  apps.
- Menu and tooltip hints still read "Ctrl+" and "Alt+"; read them as Cmd
  and Option.
- **Delete** is the key above Return (it deletes backward). **Forward
  Delete** is fn+Delete on keyboards without a separate key.
- **Enter** in these tables is the Return key (or the keypad's Enter).
- Home, End, PageUp and PageDown are fn+Left, fn+Right, fn+Up and
  fn+Down on laptop keyboards.
- The function keys (F2, F3, F5, F9, F11, F12) may need fn, depending on
  the **Keyboard** setting in System Settings. macOS takes some for itself
  (F11 shows the desktop) unless they are turned off there.
- Cmd+H and Cmd+Q belong to the macOS app menu (Hide and Quit), so Cmd+H
  doesn't open Replace.
- In the WordLite page, Cmd+Left / Cmd+Right move by word and Cmd+Delete
  deletes a word (other Mac apps use Option for these); Cmd+Left is not
  the start of the line.
  In text boxes (dialogs, Find, the formula bar) Option+arrows and
  Option+Delete work by word, as usual.

## Both applications

### Window and files

| Keys | Action |
| --- | --- |
| Cmd+N | New window (a blank workbook or document) |
| Cmd+O | File page, Open |
| Cmd+F12 | Open file dialog |
| Cmd+S | Save |
| F12 | File page, Save As |
| Cmd+P | File page, Print (export to PDF) |
| Cmd+W | Close the window (asks about unsaved changes) |
| Cmd+Q | Quit (macOS app menu) |
| Cmd+H | Hide the app (macOS app menu) |
| Esc | Close the topmost menu, drop-down, dialog or the File page; then cancel whatever is in progress (see each app) |

### Menus, dialogs and panes

- The menu bar, ribbon drop-downs, color picker, Undo/Redo history lists
  and right-click menus are mouse-only. Esc closes an open menu-bar menu
  and right-click menu. There is no keyboard access to them.
- In dialogs, Tab / Shift+Tab move between controls, and Enter or Space
  presses the focused button. Enter in a dialog's text box usually means OK,
  and Esc means Cancel. Exceptions are listed under each app.
- Message boxes (for example "Save changes?") take Esc for Cancel; pick
  the other answers with the mouse.
- Text boxes (dialogs, Find, the formula bar) have the usual editing keys:
  arrows, Home/End, Shift to select, Cmd+A/C/X/V/Z, Option+Delete.
- The Open and Save As dialogs are the system's own, with its keys.

### Scrolling

| Mouse | Action |
| --- | --- |
| Wheel | Scroll |
| Shift+wheel | Scroll sideways |
| Cmd+wheel | Zoom in or out, 10% per step |
| Shift+click in a scroll bar's track | Jump the thumb to the pointer |
| Hold a scroll bar arrow or the track | Keep scrolling |

## ExcelLite

### Workbook

| Keys | Action |
| --- | --- |
| Cmd+Option+= or Cmd+Option++ | Zoom in 10% |
| Cmd+Option+- | Zoom out 10% |
| Cmd+0 | Zoom to 100% |
| F9 | Recalculate all formulas |
| Shift+F11 | Insert a worksheet |
| F11 | Insert a chart sheet |
| Option+F1 | Insert a clustered column chart of the data |
| Cmd+PageDown / Cmd+PageUp | Next / previous sheet (wraps around) |

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
| Cmd+arrows | Jump to the edge of the data block |
| Cmd+Shift+arrows | Extend the selection to the edge of the data block |
| Tab / Shift+Tab | Move right / left |
| Enter / Shift+Enter | Move down / up |
| PageDown / PageUp | Move one screen down / up (Shift extends) |
| Home | First column of the row |
| Cmd+Home | Top-left cell (below and right of frozen panes) |
| Cmd+End | Last cell of the used range |
| Shift with Home, Cmd+Home or Cmd+End | Extend the selection there |
| Cmd+A | Select the whole sheet |

Enter and Tab collapse a multi-cell selection; they don't step through it
as in Excel. End on its own does nothing (there is no End mode).

### Entering and editing cells

| Keys | Action |
| --- | --- |
| Typing | Replace the cell's content (Enter mode) |
| F2 | Edit the cell's content (Edit mode); while editing, switch between Enter and Edit mode |
| Delete (grid) | Clear the cell and start an empty entry |
| Forward Delete (grid) | Clear contents (formats stay) |
| Enter / Shift+Enter | Commit and move down / up |
| Tab / Shift+Tab | Commit and move right / left |
| Cmd+Enter | Commit and stay |
| Option+Enter | Line break in the cell |
| Arrows | Enter mode: commit and move. Edit mode and in the formula bar: move the caret |
| Esc | Cancel the entry |
| Shift+F3 | Insert Function (also works while editing) |

While a cell is being edited, the grid, formatting and Insert shortcuts
are off, except Shift+F3.

### Formatting (grid)

| Keys | Action |
| --- | --- |
| Cmd+B / Cmd+I / Cmd+U | Bold / italic / underline |
| Cmd+5 | Strikethrough |
| Cmd+1 | Format Cells |
| Cmd+Shift+~ | General format |
| Cmd+Shift+$ | Currency (`$#,##0.00`) |
| Cmd+Shift+% | Percent |
| Cmd+Shift+# | Date (`d-mmm-yy`) |
| Cmd+Shift+@ | Time (`h:mm AM/PM`) |
| Cmd+Shift+! | Number (`#,##0.00`) |
| Esc | Stop the Format Painter |

### Clipboard, undo and structure (grid)

| Keys | Action |
| --- | --- |
| Cmd+C / Cmd+X / Cmd+V | Copy / cut / paste |
| Cmd+Z | Undo |
| Cmd+Y | Redo (Cmd+Shift+Z isn't bound) |
| Cmd+D / Cmd+R | Fill down / right |
| Cmd++ or Cmd+Shift+= | Insert cells, rows or columns |
| Cmd+- | Delete cells, rows or columns |
| Shift+Option+Right / Shift+Option+Left | Group / ungroup (outline) |

### Find, Go To and Insert

| Keys | Action |
| --- | --- |
| Cmd+F | Find |
| Cmd+H | Replace: taken by Hide, so use **Home > Replace** |
| Cmd+G or F5 | Go To |
| Cmd+K | Insert hyperlink |
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
| Cmd+Shift+L | AutoFilter on / off |
| Option+Down | On a filter header: open its filter drop-down. Elsewhere: open the cell's validation list |

- Filter and PivotTable field drop-downs: Enter in the search box applies
  the checklist like OK; Esc closes.
- Validation list: Up / Down (or Option+Down) move, Enter or Tab picks, Esc
  closes.

### Charts

These keys work while a chart is selected.

| Keys | Action |
| --- | --- |
| Arrows | Nudge the chart 1 pixel |
| Cmd+arrows | Nudge the chart a quarter pixel |
| Delete / Forward Delete | Delete the selected part (title, legend, axis, series...) or the chart |
| F2 or Enter, title selected | Edit the title (Enter commits, Esc abandons) |
| Typing, title selected | Replace the title |
| Cmd+C / Cmd+X / Cmd+V | Copy / cut / paste the chart |
| Cmd+Z / Cmd+Y | Undo / redo |
| Esc | Chart part selected: select the whole chart; then deselect it |

Other grid keys (Tab, Home, Cmd+A and so on) deselect the chart and act
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
| Cmd+click or Cmd+drag | Add another range to the selection |
| Double-click a cell | Edit it (on a PivotTable value: Show Details) |
| Cmd+click a hyperlink | Follow it |
| Drag the selection border | Move the cells; hold Cmd when releasing to copy |
| Drag the fill handle | Fill a series or copy |
| Drag a header divider | Resize the column or row |
| Double-click a header divider | AutoFit the column or row |
| Click or drag while typing a formula | Insert a reference (Shift+click extends it) |
| Hover a cell with a note | Show the note |
| Right-click or Control+click | Context menu (chart menu over a chart) |
| Click the formula bar | Edit the cell there |
| Double-click a sheet tab | Rename it |
| Drag a chart / its handles | Move / resize it; Option snaps to cell edges, Shift keeps a corner's proportions |
| Double-click a chart title | Edit it |
| Format Painter: click / double-click | Paint once / keep painting until Esc |

## WordLite

### Window and view

| Keys | Action |
| --- | --- |
| Cmd++ or Cmd+= | Zoom in 10% |
| Cmd+- | Zoom out 10% |
| Cmd+0 | Zoom to 100% |

Esc closes the topmost dialog, menu or the File page. If none is open, it
cancels a text move or table-border drag, closes the right-click menu,
turns off the Format Painter, closes the Find bar, or leaves the header or
footer. The focus then returns to the page.

### Moving and selecting (page)

Add Shift to any of these to extend the selection.

| Keys | Action |
| --- | --- |
| Left / Right | One character (a plain arrow collapses a selection to that side) |
| Cmd+Left / Cmd+Right | One word |
| Up / Down | One line |
| Cmd+Up / Cmd+Down | Previous / next paragraph |
| Home / End | Start / end of the line |
| Cmd+Home / Cmd+End | Start / end of the document |
| PageUp / PageDown | One screen |
| Cmd+A | Select all |

### Editing (page)

| Keys | Action |
| --- | --- |
| Delete / Forward Delete | Delete back / forward (or the selection) |
| Cmd+Delete / Cmd+Forward Delete | Delete the previous / next word |
| Enter | New paragraph |
| Shift+Enter | Line break |
| Cmd+Enter | Page break |
| Tab | Table: next cell. Start of a list item: demote. Otherwise: a tab |
| Shift+Tab | Table: previous cell. Start of a list item: promote |
| Cmd+Tab | A tab character, even in a table |
| Cmd+C / Cmd+X / Cmd+V | Copy / cut / paste |
| Cmd+Option+V | Paste Special |
| Cmd+Z | Undo |
| Cmd+Y or Cmd+Shift+Z | Redo |

In a table, Tab in the last cell adds a row.

### Formatting

| Keys | Action |
| --- | --- |
| Cmd+B / Cmd+I / Cmd+U | Bold / italic / underline (page) |
| Cmd+L / Cmd+E / Cmd+R / Cmd+J | Align left / center / right / justify (page) |
| Cmd+Space | Clear character formatting (page) |
| Cmd+Shift+N | Normal style (page) |
| Cmd+Option+1 / 2 / 3 | Heading 1 / 2 / 3 (page) |
| Cmd+Shift+> / Cmd+Shift+< | Grow / shrink the font |
| Cmd+Shift+C / Cmd+Shift+V | Copy / paste formatting |
| Cmd+Shift+8 | Show/Hide ¶ |
| Cmd+Option+Shift+S | Styles pane on / off |

### Find, fields and insert

| Keys | Action |
| --- | --- |
| Cmd+F | Find bar |
| Cmd+H | Replace: taken by Hide, so use **Find & Replace** on the Home ribbon |
| Cmd+G | Go to page |
| F3 / Shift+F3 | Next / previous match (F3 with an empty Find box opens Find) |
| F9 | Update the fields in the selection (and the table of contents) |
| Option+F9 | Show / hide field codes |
| Cmd+F9 | Field dialog |
| Cmd+K | Insert or edit a link (page) |
| Cmd+Option+M | New comment (page) |

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
| Left margin: triple-click or Cmd+click | Select the document |
| Left margin: Shift+click or drag | Extend by lines |
| Drag the selection | Move it; hold Cmd when releasing to copy; Esc cancels |
| Cmd+click a link | Open it |
| Click a picture / drag its handles | Select / resize it |
| Drag a table border | Resize the column or row (Esc cancels) |
| Double-click the header or footer | Edit it; double-click the body to leave |
| Right-click or Control+click | Context menu |
| Format Painter: click / double-click | Paint once / keep painting until Esc |
| Styles pane: click / double-click | Select / apply a style |

## Differences from Excel and Word

Keys that do something different from Microsoft Office, or aren't bound:

- **Zoom.** ExcelLite zooms with Cmd+Option+= / Cmd+Option+- and WordLite with
  Cmd+= / Cmd+-. Cmd+0 resets the zoom in both. In Office, Cmd+0 hides
  columns, and Cmd+= / Cmd+Shift+= are subscript / superscript in Word.
  WordLite has no subscript or superscript keys (use the ribbon).
- **ExcelLite.** Cmd+A selects the whole sheet at once. Enter and Tab
  don't step through a selection. Cmd+Enter in the grid moves down
  instead of filling the selection. Cmd+PageDown wraps to the first sheet.
  End mode, F4, Cmd+; (date), Cmd+Shift+; (time), Cmd+Space,
  Shift+Space, Cmd+` (show formulas), Option+= (AutoSum), Cmd+T, Cmd+9,
  Cmd+Shift+6, Cmd+Shift+7 and Cmd+Shift+Z are not bound.
- **WordLite.** Shift+F3 is find previous (Word: Change Case).
  Cmd+Shift+Enter is a page break (Word: column break). Shift+F9 isn't
  bound. Cmd+PageUp/PageDown act like PageUp/PageDown.
- **Menus.** There is no keyboard access to the menus or ribbon (no
  KeyTips, no Control+F2 to reach the menu bar), so they need the mouse.
