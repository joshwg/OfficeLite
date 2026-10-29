// Package version holds the OfficeLite release number.
package version

// Version is the OfficeLite software version.
const Version = "0.7.1"

// Product is the suite name shown in About text.
const Product = "OfficeLite"

// String is the product and version, e.g. "OfficeLite 1.2.3".
func String() string { return Product + " " + Version }
