// Package limits holds the limits of the shared packages under common/
// and of the commands under cmd/: size, length and count caps, read and
// allocation caps that keep damaged or hostile files from exhausting
// memory, retry counts, timeouts, polling intervals and delays. Each
// constant is named for the area it belongs to (AutoSave…, Recovery…,
// IWork…, PDF…, …). It imports nothing from officelite, so any package
// can use it.
package limits

import "time"

// ---- atomicfile: replacing files whole ----

const (
	// AtomicWriteBuffer is the size of the buffer between an
	// atomicfile.Write's fill function and the temporary file: 256 KB, so
	// that a large document goes out in few system calls.
	AtomicWriteBuffer = 256 << 10

	// RenameRetries is how many times atomicfile retries the final rename
	// on Windows, where a sharing violation (another process reading the
	// target) passes quickly. The i-th retry waits i×RenameBackoff, about a
	// third of a second in all.
	RenameRetries = 8

	// RenameBackoff is the step of the growing wait between RenameRetries.
	RenameBackoff = 10 * time.Millisecond
)

// ---- autorecover: recovery copies ----

const (
	// RecoveryMaxAge is how long a recovery copy nobody dealt with is kept.
	RecoveryMaxAge = 30 * 24 * time.Hour

	// RecoveryStrayAge is how old a copy without a sidecar (or a temporary
	// file) must be to be cleaned up: a writer may be between the two
	// files.
	RecoveryStrayAge = 24 * time.Hour

	// RecoveryCheckEvery is how often the AutoRecover timer looks at the
	// interval setting, so that a change to it takes effect soon.
	RecoveryCheckEvery = 30 * time.Second

	// RecoveryWaitPoll is how often Saver.Wait (tests) looks whether the
	// saver's queue has drained.
	RecoveryWaitPoll = 5 * time.Millisecond

	// RecoveryMaxStem is the most characters of the document's name kept
	// in a recovery copy's file name, keeping the name (with its random
	// suffix and extension) well short of what any file system allows.
	RecoveryMaxStem = 60

	// RecoveryClaimStale is how old a recovery entry's claim file must be
	// to be taken over when it names no process (its claimer crashed
	// between creating it and writing its pid). A claimer holds the file
	// only while it re-checks and rewrites one sidecar.
	RecoveryClaimStale = 10 * time.Second

	// RecoveryClaimRemoveRetries is how many times a claimer retries
	// removing its claim file on Windows, where another claimer reading it
	// (to see whether it is stale) briefly blocks deletion. The i-th retry
	// waits i×RecoveryClaimRemoveBackoff, about a third of a second in all.
	RecoveryClaimRemoveRetries = 8

	// RecoveryClaimRemoveBackoff is the step of the growing wait between
	// RecoveryClaimRemoveRetries.
	RecoveryClaimRemoveBackoff = 10 * time.Millisecond
)

// ---- autosave: saving a document into its own file as it is edited ----

const (
	// AutoSaveDelay is how long after the last edit the document is saved.
	AutoSaveDelay = 3 * time.Second

	// AutoSaveMaxWait is the longest edits wait while more keep coming.
	AutoSaveMaxWait = 60 * time.Second

	// AutoSaveShown is how long a window's title says "Saved" after an
	// AutoSave.
	AutoSaveShown = 4 * time.Second
)

// ---- cmd/officelite ----

// ExitFlushTimeout is how long the program, once its last window closes,
// waits for each of AutoRecover's deletions and AutoSave's saves still
// being written before it exits, so that exiting doesn't cut one short
// but a stuck disk can't keep the process alive.
const ExitFlushTimeout = 10 * time.Second

// ---- gui: lists and counts ----

const (
	// DropFiles is how many files one drop onto a window opens; Office
	// asks before opening many, OfficeLite opens the first ones and says
	// so.
	DropFiles = 20

	// DropBadNamed is how many of the dropped files that can't be opened
	// are named in the message; the rest are counted ("4 more").
	DropBadNamed = 3

	// HomeRecentFiles is how many recent files the backstage's Home page
	// lists; Open lists them all.
	HomeRecentFiles = 12

	// HistoryEntries is the most entries an undo/redo HistoryList shows.
	HistoryEntries = 100

	// RecentColors is how many custom colors the color picker's Recent
	// Colors row keeps: one row of swatches, as in Office.
	RecentColors = 10
)

// ---- gui: timing ----

const (
	// TipDelay is how long the pointer rests on a control before its
	// tooltip shows.
	TipDelay = 500 * time.Millisecond

	// TipTimeout is how long a tooltip stays up.
	TipTimeout = 10 * time.Second

	// TipWarm is how soon after a shown tip another control's tip shows
	// without the delay (switch without delay within this of a shown tip).
	TipWarm = 300 * time.Millisecond

	// ScrollRepeatDelay is how long an arrow or the track of a scrollbar
	// is held before it auto-repeats, as Windows does.
	ScrollRepeatDelay = 400 * time.Millisecond

	// ScrollRepeatEvery is the auto-repeat rate after ScrollRepeatDelay.
	ScrollRepeatEvery = 50 * time.Millisecond
)

// ---- gui: caches and buffers ----

const (
	// TextRunCacheGen is how many run images a cache generation of the
	// DirectWrite text renderer holds; images unused for a whole
	// generation are dropped.
	TextRunCacheGen = 4000

	// WindowsPathChars is the size, in UTF-16 code units, of the buffer
	// the Windows file dialogs return the chosen path in: 32768, the
	// longest extended-length path Windows allows.
	WindowsPathChars = 32768
)

// ---- fonts read from disk (gui's font library, pdf) ----

const (
	// FontCollectionMaxFonts is the most fonts a TrueType collection
	// (.ttc) may hold, so that a damaged or hostile file can't make a
	// reader loop or allocate without bound.
	FontCollectionMaxFonts = 256

	// FontNameTableMax is the largest "name" table the font library reads
	// for a font's family names, in bytes (1 MB).
	FontNameTableMax = 1 << 20
)

// ---- iwork: Numbers, Pages and Keynote files ----

const (
	// IWorkMaxZipEntry is the largest file read from inside an iWork
	// package's zip (1 GB), so that a zip bomb can't exhaust memory; a
	// larger one is an error (iwork.ErrTooLarge).
	IWorkMaxZipEntry = 1 << 30

	// IWorkSnappyMaxRatio bounds a Snappy block's claimed uncompressed
	// length by its compressed size: no element expands more than 64/3
	// times (a 3-byte copy of 64 bytes), so a longer claimed length is
	// corrupt; checking it before allocating keeps a few bad bytes from
	// reserving a gigabyte.
	IWorkSnappyMaxRatio = 22

	// IWorkMaxTableRows is Numbers' limit on a table's rows, which bounds
	// the dimensions read from a file: a damaged one could otherwise claim
	// billions of rows (or, through the int conversion, a negative count)
	// for a reader to loop over.
	IWorkMaxTableRows = 1_000_000

	// IWorkMaxTableCols is Numbers' limit on a table's columns, for the
	// same reason as IWorkMaxTableRows.
	IWorkMaxTableCols = 1_000

	// IWorkMaxDecimalPlaces is the most decimals a number format asks
	// for; a larger decimal_places (damaged or hostile) is cut to it.
	IWorkMaxDecimalPlaces = 15
)

// ---- lockfile: owner files and the open-document registry ----

const (
	// OwnerFileRead is how much of an Office owner file (~$name) is read:
	// Office's are 162 (Word) or 165 (Excel) bytes.
	OwnerFileRead = 512

	// OwnerFileStaleAfter is how old an owner file must be to count as
	// left over where the OS can't tell whether Office has it open.
	OwnerFileStaleAfter = 24 * time.Hour

	// HandOffRequestMax is the most bytes read of one hand-off request
	// (the token and a path, a line each): 8 KB, so that a stray
	// connection can't make the receiver read without bound.
	HandOffRequestMax = 8 << 10

	// HandOffTimeout is the deadline of one hand-off connection, sending
	// or answering, so that a process that hangs can't block the one
	// opening the file.
	HandOffTimeout = 2 * time.Second

	// WindowsDisplayNameChars is the size, in UTF-16 code units, of the
	// buffer the Windows user's full name is read into.
	WindowsDisplayNameChars = 256
)

// ---- pdf ----

// PDFIDHashBytes is how much of the start of a PDF's output (4 KB) is
// hashed into its file ID: enough to tell documents apart without hashing
// the whole file.
const PDFIDHashBytes = 4096

// ---- recent: the recent-files list ----

// RecentFiles is how many files the recent-files list keeps.
const RecentFiles = 50

// ---- templates ----

// TemplatesRefreshEvery is how stale a templates.Lister's list may get
// before the folder is read again.
const TemplatesRefreshEvery = 2 * time.Second
