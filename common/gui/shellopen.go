package gui

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

// errUnsafeLink is OpenURL's error for a link it won't open.
var errUnsafeLink = errors.New("links to programs and scripts aren't opened")

// executableExts are the file types Windows runs rather than opens: a
// document's link to one of them could start a program.
var executableExts = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".ps1": true, ".psm1": true,
	".psd1": true, ".ps2": true, ".psc1": true, ".vb": true, ".vbs": true, ".vbe": true,
	".js": true, ".jse": true, ".wsf": true, ".wsh": true, ".ws": true, ".sct": true,
	".msi": true, ".msp": true, ".mst": true, ".msc": true, ".msix": true, ".appx": true,
	".scr": true, ".pif": true, ".cpl": true, ".hta": true, ".lnk": true, ".url": true,
	".reg": true, ".inf": true, ".jar": true, ".dll": true, ".ocx": true, ".sys": true,
	".drv": true, ".application": true, ".appref-ms": true, ".gadget": true, ".scf": true,
	".chm": true, ".settingcontent-ms": true, ".xbap": true, ".mde": true, ".mda": true,
	".sh": true, ".command": true, ".app": true, ".desktop": true,
}

// linkAllowed reports whether OpenURL may open target: web, FTP and
// e-mail links, and files and folders (paths or file: URLs) that aren't
// programs or scripts. Other schemes (javascript:, ms-..., custom protocol
// handlers) are refused.
func linkAllowed(target string) bool {
	t := strings.TrimSpace(target)
	low := strings.ToLower(t)
	for _, p := range []string{"http://", "https://", "ftp://", "mailto:"} {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	p := t
	if strings.HasPrefix(low, "file:") {
		u, err := url.Parse(t)
		if err != nil {
			return false
		}
		p = u.Path
		if u.Host != "" && u.Host != "localhost" {
			p = "//" + u.Host + p
		}
	} else if i := strings.IndexByte(t, ':'); i > 1 {
		return false // a scheme other than the ones above
	}
	if p == "" {
		return false
	}
	name := path.Base(strings.ReplaceAll(p, `\`, "/"))
	// Windows ignores trailing dots and spaces ("calc.exe. "), and a colon
	// in the name selects an alternate data stream: refuse rather than
	// guess what would run.
	name = strings.TrimRight(name, ". ")
	if strings.Contains(name, ":") {
		return false
	}
	if executableExts[strings.ToLower(path.Ext(name))] {
		return false
	}
	return !isExecutableFile(p)
}
