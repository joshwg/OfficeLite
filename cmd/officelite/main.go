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

	"gioui.org/app"

	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/version"
	excelui "officelite/excel/ui"
	wordui "officelite/word/ui"
)

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
	// open starts a window; windows call it for New, Open and recent files.
	var open func(kind doctype.Kind, path string)
	open = func(kind doctype.Kind, path string) {
		windows.Add(1)
		go func() {
			defer windows.Done()
			w := new(app.Window)
			var err error
			if kind == doctype.Document {
				err = wordui.Run(w, wordui.Config{Path: path, Fonts: fonts, Launch: open})
			} else {
				err = excelui.Run(w, excelui.Config{Path: path, Fonts: fonts, Launch: open})
			}
			if err != nil {
				log.Print(err)
			}
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
			// Unreadable or unrecognised files go to the spreadsheet window,
			// which reports load errors and tries unknown text as CSV.
			kind, _ = doctype.DetectFile(path)
		}
		open(kind, path)
	}

	go func() {
		windows.Wait()
		os.Exit(0)
	}()
	app.Main()
}
