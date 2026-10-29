// Command officelite opens spreadsheets (ExcelLite) and documents
// (WordLite). Each file gets its own window; all windows share one process
// and one font library, and the program exits when the last one closes.
//
//	officelite [--excel | --word] [file ...]
//	officelite --version
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"gioui.org/app"

	"officelite/common/autorecover"
	"officelite/common/autosave"
	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/common/version"
	excelui "officelite/excel/ui"
	wordui "officelite/word/ui"
)

// main parses the flags, offers any crashed session's copies for recovery,
// opens a window per file (or one blank one) and runs until the last
// window closes.
func main() {
	excel := flag.Bool("excel", false, "open files as spreadsheets; with no files, start a blank workbook")
	word := flag.Bool("word", false, "open files as documents; with no files, start a blank document")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: officelite [--excel | --word] [file ...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}

	fonts := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	var windows sync.WaitGroup
	// Recovery copies left by an OfficeLite that crashed, found before
	// this one's windows write their own.
	orphans := autorecover.Orphans("")
	// open starts a window; windows call it for New, Open and recent files.
	// A file already open in a window (of this process or another
	// OfficeLite's) brings that window to the front instead.
	var open func(kind doctype.Kind, path string)
	// start opens a window for a file or, with rec, a recovered copy.
	start := func(kind doctype.Kind, path string, rec *autorecover.Entry) {
		windows.Add(1)
		go func() {
			defer windows.Done()
			w := new(app.Window)
			var err error
			if kind == doctype.Document {
				err = wordui.Run(w, wordui.Config{Path: path, Fonts: fonts, Launch: open, Recovered: rec})
			} else {
				err = excelui.Run(w, excelui.Config{Path: path, Fonts: fonts, Launch: open, Recovered: rec})
			}
			if err != nil {
				log.Print(err)
			}
		}()
	}
	open = func(kind doctype.Kind, path string) {
		if path != "" && lockfile.Activate(path) {
			return
		}
		start(kind, path, nil)
	}
	if len(orphans) > 0 {
		windows.Add(1)
		go func() {
			defer windows.Done()
			runRecovery(new(app.Window), fonts, orphans, func(e autorecover.Entry) {
				kind := doctype.Spreadsheet
				if e.App == autorecover.Word {
					kind = doctype.Document
				}
				start(kind, "", &e)
			})
		}()
	}

	forced := doctype.Unknown
	switch {
	case *excel:
		forced = doctype.Spreadsheet
	case *word:
		forced = doctype.Document
	}
	if flag.NArg() == 0 {
		kind := doctype.Spreadsheet
		if forced == doctype.Document {
			kind = doctype.Document
		}
		open(kind, "")
	}
	for _, path := range flag.Args() {
		kind := forced
		if kind == doctype.Unknown {
			// Unreadable or unrecognized files go to the spreadsheet window,
			// which reports load errors and tries unknown text as CSV.
			kind, _ = doctype.DetectFile(path)
		}
		open(kind, path)
	}

	// With every file handed to the OfficeLite that has it open, there is
	// no window and the program ends here.
	go func() {
		windows.Wait()
		autorecover.Flush(10 * time.Second) // the closed windows' copies deleted
		autosave.Flush(10 * time.Second)    // and their AutoSaves written
		os.Exit(0)
	}()
	app.Main()
}
