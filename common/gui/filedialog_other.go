//go:build !windows

package gui

import "gioui.org/io/event"

func (d *FileDialog) listen(event.Event) {}

func (d *FileDialog) open([]FileFilter) (string, error) { return "", ErrNoFileDialog }

func (d *FileDialog) save(string, []FileFilter) (string, error) { return "", ErrNoFileDialog }
