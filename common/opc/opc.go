// Package opc has the part-name helpers of Open Packaging Conventions
// packages (.docx, .xlsx): relationship parts and target resolution.
package opc

import (
	"path"
	"strings"
)

// RelsPath is the relationships part of a part ("" = the package):
// word/document.xml → word/_rels/document.xml.rels.
func RelsPath(part string) string {
	dir, base := path.Split(part)
	return dir + "_rels/" + base + ".rels"
}

// Resolve turns a relationship target of part from into a part name
// (without the leading '/'): relative targets are relative to from's
// folder, absolute ones to the package root.
func Resolve(from, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	return strings.TrimPrefix(path.Clean(path.Join(path.Dir(from), target)), "/")
}
