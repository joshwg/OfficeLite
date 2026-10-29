package gui

import (
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
)

var (
	getOpenFileName = syscall.NewLazyDLL("comdlg32.dll").NewProc("GetOpenFileNameW")
	getSaveFileName = syscall.NewLazyDLL("comdlg32.dll").NewProc("GetSaveFileNameW")
)

// openFileName is OPENFILENAMEW.
type openFileName struct {
	StructSize      uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	FnHook          uintptr
	TemplateName    *uint16
	PvReserved      uintptr
	DwReserved      uint32
	FlagsEx         uint32
}

const (
	ofnOverwritePrompt = 0x00000002
	ofnHideReadOnly    = 0x00000004
	ofnNoChangeDir     = 0x00000008
	ofnPathMustExist   = 0x00000800
	ofnFileMustExist   = 0x00001000
	ofnExplorer        = 0x00080000
	ofnForceShowHidden = 0x10000000
)

func (d *FileDialog) listen(e event.Event) {
	if v, ok := e.(app.Win32ViewEvent); ok {
		d.owner.Store(v.HWND)
	}
}

// filterSpec is the dialog's filter: "label\0pattern\0" pairs ending in
// an extra \0, or nil (no filter) when there are none: a lone \0 isn't
// double-terminated, and the dialog would read past it.
func filterSpec(filters []FileFilter) *uint16 {
	if len(filters) == 0 {
		return nil
	}
	var spec []rune
	for _, f := range filters {
		spec = append(spec, []rune(f.Label())...)
		spec = append(spec, 0)
		spec = append(spec, []rune(f.Pattern())...)
		spec = append(spec, 0)
	}
	spec = append(spec, 0)
	return &utf16.Encode(spec)[0]
}

func (d *FileDialog) open(filters []FileFilter) (string, error) {
	filter := filterSpec(filters)
	path := make([]uint16, 32768)
	ofn := openFileName{
		Owner:       d.owner.Load(),
		Filter:      filter,
		FilterIndex: 1,
		File:        &path[0],
		MaxFile:     uint32(len(path)),
		Flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly | ofnNoChangeDir | ofnForceShowHidden,
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := getOpenFileName.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return "", ErrCancelled
	}
	return syscall.UTF16ToString(path), nil
}

func (d *FileDialog) save(name string, filters []FileFilter) (string, error) {
	filter := filterSpec(filters)
	path := make([]uint16, 32768)
	copy(path[:len(path)-1], utf16.Encode([]rune(name)))
	ofn := openFileName{
		Owner:       d.owner.Load(),
		Filter:      filter,
		FilterIndex: 1,
		File:        &path[0],
		MaxFile:     uint32(len(path)),
		Flags:       ofnExplorer | ofnOverwritePrompt | ofnPathMustExist | ofnHideReadOnly | ofnNoChangeDir,
	}
	if len(filters) > 0 && len(filters[0].Exts) > 0 {
		ext := utf16.Encode([]rune(strings.TrimPrefix(filters[0].Exts[0], ".") + "\x00"))
		ofn.DefExt = &ext[0]
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := getSaveFileName.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return "", ErrCancelled
	}
	return syscall.UTF16ToString(path), nil
}
