// FileDialog stubs for platforms without a native dialog: Open and Save
// return ErrNoFileDialog.

//go:build !windows

package gui

import "gioui.org/io/event"

// listen does nothing: there's no native owner window to record.
func (d *FileDialog) listen(event.Event) {}

// open returns ErrNoFileDialog.
func (d *FileDialog) open([]FileFilter) (string, error) { return "", ErrNoFileDialog }

// save returns ErrNoFileDialog.
func (d *FileDialog) save(string, []FileFilter) (string, error) { return "", ErrNoFileDialog }
