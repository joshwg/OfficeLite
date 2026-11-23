// Tests of officeweb's start-up checks: the data folder kept apart from
// the client folder the server sends files from.

package main

import (
	"path/filepath"
	"testing"
)

// TestApart checks that a data folder inside, holding or equal to the
// client folder is refused, and that side-by-side folders (even with
// names sharing a prefix) are fine.
func TestApart(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	for _, data := range []string{filepath.Join(root, "data"), filepath.Join(root, "client-data")} {
		if err := apart(data, client); err != nil {
			t.Errorf("apart(%s): %v", data, err)
		}
	}
	for _, data := range []string{client, filepath.Join(client, "data"), root} {
		if apart(data, client) == nil {
			t.Errorf("apart(%s) accepted", data)
		}
	}
}
