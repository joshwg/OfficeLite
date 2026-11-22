// The desktop has no remote list: it is kept in a file (recent.go).

//go:build !js

package recent

// defaultRemote is nil on the desktop.
func defaultRemote() remote { return nil }
