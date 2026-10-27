// Package gui holds Gio helpers shared by the Office apps: fonts, painting
// primitives and small widgets.
package gui

import (
	"strings"

	"gioui.org/font"
	"gioui.org/font/gofont"
)

// metricCompatible lists free fonts with the same metrics as common Office
// fonts, so layout matches when the Microsoft font is not installed (Linux).
var metricCompatible = map[string]string{
	"calibri":         "Carlito",
	"cambria":         "Caladea",
	"arial":           "Liberation Sans, Arimo",
	"helvetica":       "Liberation Sans, Arimo",
	"times new roman": "Liberation Serif, Tinos",
	"courier new":     "Liberation Mono, Cousine",
	"aptos":           "Carlito",
	"aptos narrow":    "Arial Narrow, Liberation Sans Narrow",
}

// Typeface returns a Gio typeface list for an Office font name: the font
// itself, metric-compatible substitutes, then a generic sans-serif.
func Typeface(name string) font.Typeface {
	list := strings.ReplaceAll(name, ",", " ")
	if fb, ok := metricCompatible[strings.ToLower(name)]; ok {
		list += ", " + fb
	}
	return font.Typeface(list + ", " + strings.Join(FallbackFamilies, ", ") + ", Go")
}

func gofontCollection() []font.FontFace { return gofont.Collection() }

// UIFont is the font used for application chrome (headers, tabs, bars).
var UIFont = font.Font{Typeface: "Segoe UI, Tahoma, Calibri, Carlito, Liberation Sans, sans-serif"}
