package engineering

import (
	"math"

	f "officelite/excel/formula"
)

// The unit classes CONVERT converts within.
const (
	mass = iota
	distance
	duration
	pressure
	force
	energy
	power
	magnetism
	temperature
	volume
	area
	information
	speed
)

// A unit is a size in its class's base unit (gram, meter, second, pascal,
// newton, joule, watt, tesla, kelvin, cubic meter, square meter, bit,
// meter per second).
type unit struct {
	class  int
	factor float64
	prefix bool // takes metric (and, for information, binary) prefixes
	power  int  // 2 or 3 for square and cubic units: a prefix applies squared or cubed
}

// Exact definitions of the customary units.
const (
	inch    = 0.0254
	foot    = 12 * inch
	yard    = 3 * foot
	mile    = 5280 * foot
	nmile   = 1852.0
	pica    = inch / 6
	picapt  = inch / 72
	surveyM = 6336000.0 / 3937 // U.S. survey mile
	lightYr = 9460730472580800.0
	gallon  = 231 * inch * inch * inch
	ukGal   = 0.00454609
	pound   = 453.59237 // gram
	gravity = 9.80665
	lbf     = pound / 1000 * gravity
	hp      = 550 * foot * lbf
	atm     = 101325.0
)

var units = map[string]unit{}

func init() {
	add := func(class int, factor float64, prefix bool, names ...string) {
		for _, n := range names {
			units[n] = unit{class: class, factor: factor, prefix: prefix, power: 1}
		}
	}
	// addPow adds a square (p = 2) or cubic (p = 3) unit under its names
	// with and without "^" (ft2, ft^2).
	addPow := func(class, p int, factor float64, prefix bool, names ...string) {
		for _, n := range names {
			for _, s := range []string{n + string(rune('0'+p)), n + "^" + string(rune('0'+p))} {
				units[s] = unit{class: class, factor: factor, prefix: prefix, power: p}
			}
		}
	}

	add(mass, 1, true, "g")
	add(mass, lbf/foot*1000, false, "sg")
	add(mass, pound, false, "lbm")
	add(mass, 1.660538782e-24, true, "u")
	add(mass, pound/16, false, "ozm")
	add(mass, pound/7000, false, "grain")
	add(mass, 100*pound, false, "cwt", "shweight")
	add(mass, 112*pound, false, "uk_cwt", "lcwt", "hweight")
	add(mass, 14*pound, false, "stone")
	add(mass, 2000*pound, false, "ton")
	add(mass, 2240*pound, false, "uk_ton", "LTON", "brton")

	add(distance, 1, true, "m")
	add(distance, mile, false, "mi")
	add(distance, nmile, false, "Nmi")
	add(distance, inch, false, "in")
	add(distance, foot, false, "ft")
	add(distance, yard, false, "yd")
	add(distance, 1e-10, true, "ang")
	add(distance, 45*inch, false, "ell")
	add(distance, lightYr, true, "ly")
	add(distance, 3.0856775814913673e16, true, "parsec", "pc")
	add(distance, picapt, false, "Picapt", "Pica")
	add(distance, pica, false, "pica")
	add(distance, surveyM, false, "survey_mi")

	add(duration, 365.25*86400, false, "yr")
	add(duration, 86400, false, "day", "d")
	add(duration, 3600, false, "hr")
	add(duration, 60, false, "mn", "min")
	add(duration, 1, true, "sec", "s")

	add(pressure, 1, true, "Pa", "p")
	add(pressure, atm, true, "atm", "at")
	add(pressure, atm/760, true, "mmHg")
	add(pressure, lbf/(inch*inch), false, "psi")
	add(pressure, atm/760, false, "Torr")

	add(force, 1, true, "N")
	add(force, 1e-5, true, "dyn", "dy")
	add(force, lbf, false, "lbf")
	add(force, gravity/1000, true, "pond")

	add(energy, 1, true, "J")
	add(energy, 1e-7, true, "e")
	add(energy, 4.184, true, "c")
	add(energy, 4.1868, true, "cal")
	add(energy, 1.602176487e-19, true, "eV", "ev")
	add(energy, hp*3600, false, "HPh", "hh")
	add(energy, 3600, true, "Wh", "wh")
	add(energy, foot*lbf, false, "flb")
	add(energy, 1055.05585262, false, "BTU", "btu")

	add(power, hp, false, "HP", "h")
	add(power, 75*gravity, false, "PS")
	add(power, 1, true, "W", "w")

	add(magnetism, 1, true, "T")
	add(magnetism, 1e-4, true, "ga")

	// Temperatures convert with the offsets in tempScale; the factor
	// is unused.
	add(temperature, 1, false, "C", "cel", "F", "fah", "Rank", "Reau")
	add(temperature, 1, true, "K", "kel")

	add(volume, gallon/768, false, "tsp")
	add(volume, 5e-6, false, "tspm")
	add(volume, gallon/256, false, "tbs")
	add(volume, gallon/128, false, "oz")
	add(volume, gallon/16, false, "cup")
	add(volume, gallon/8, false, "pt", "us_pt")
	add(volume, ukGal/8, false, "uk_pt")
	add(volume, gallon/4, false, "qt")
	add(volume, ukGal/4, false, "uk_qt")
	add(volume, gallon, false, "gal")
	add(volume, ukGal, false, "uk_gal")
	add(volume, 1e-3, true, "l", "L", "lt")
	add(volume, 42*gallon, false, "barrel")
	add(volume, 2150.42*inch*inch*inch, false, "bushel")
	add(volume, 100*foot*foot*foot, false, "GRT", "regton")
	add(volume, 40*foot*foot*foot, false, "MTON")
	addPow(volume, 3, 1e-30, true, "ang")
	addPow(volume, 3, foot*foot*foot, false, "ft")
	addPow(volume, 3, inch*inch*inch, false, "in")
	addPow(volume, 3, lightYr*lightYr*lightYr, false, "ly")
	addPow(volume, 3, 1, true, "m")
	addPow(volume, 3, mile*mile*mile, false, "mi")
	addPow(volume, 3, yard*yard*yard, false, "yd")
	addPow(volume, 3, nmile*nmile*nmile, false, "Nmi")
	addPow(volume, 3, picapt*picapt*picapt, false, "Picapt", "Pica")

	add(area, 4840*yard*yard, false, "uk_acre")
	add(area, 43560*(surveyM/5280)*(surveyM/5280), false, "us_acre")
	add(area, 100, true, "ar")
	add(area, 10000, false, "ha")
	add(area, 2500, false, "Morgen")
	addPow(area, 2, 1e-20, true, "ang")
	addPow(area, 2, foot*foot, false, "ft")
	addPow(area, 2, inch*inch, false, "in")
	addPow(area, 2, lightYr*lightYr, false, "ly")
	addPow(area, 2, 1, true, "m")
	addPow(area, 2, mile*mile, false, "mi")
	addPow(area, 2, nmile*nmile, false, "Nmi")
	addPow(area, 2, picapt*picapt, false, "Picapt", "Pica")
	addPow(area, 2, yard*yard, false, "yd")

	add(information, 1, true, "bit")
	add(information, 8, true, "byte")

	add(speed, 6080*foot/3600, false, "admkn")
	add(speed, nmile/3600, false, "kn")
	add(speed, 1.0/3600, true, "m/h", "m/hr")
	add(speed, 1, true, "m/s", "m/sec")
	add(speed, mile/3600, false, "mph")
}

// metricPrefixes are the decimal exponents of the metric prefixes ("da"
// and "e" are both deka).
var metricPrefixes = map[string]int{
	"Y": 24, "Z": 21, "E": 18, "P": 15, "T": 12, "G": 9, "M": 6, "k": 3, "h": 2, "da": 1, "e": 1,
	"d": -1, "c": -2, "m": -3, "u": -6, "n": -9, "p": -12, "f": -15, "a": -18, "z": -21, "y": -24,
}

// binaryPrefixes are the binary exponents of the binary prefixes, which
// only information units take.
var binaryPrefixes = map[string]int{
	"ki": 10, "Mi": 20, "Gi": 30, "Ti": 40, "Pi": 50, "Ei": 60, "Zi": 70, "Yi": 80,
}

// A scaled unit is a unit with a prefix: 10^dec * 2^bin of it.
type scaled struct {
	unit
	name     string
	dec, bin int
}

// lookup finds a unit name, case-sensitively, with an optional prefix.
// A name without a prefix takes precedence.
func lookup(s string) (scaled, bool) {
	if u, ok := units[s]; ok {
		return scaled{unit: u, name: s}, true
	}
	for n := 1; n <= 2 && n < len(s); n++ {
		u, ok := units[s[n:]]
		if !ok || !u.prefix {
			continue
		}
		if e, ok := metricPrefixes[s[:n]]; ok {
			return scaled{unit: u, name: s[n:], dec: e * u.power}, true
		}
		if e, ok := binaryPrefixes[s[:n]]; ok && u.class == information {
			return scaled{unit: u, name: s[n:], bin: e}, true
		}
	}
	return scaled{}, false
}

// tempScale gives each temperature unit as Celsius = (x + offset) / d.
var tempScale = map[string]struct{ offset, d float64 }{
	"C": {0, 1}, "cel": {0, 1},
	"F": {-32, 1.8}, "fah": {-32, 1.8},
	"K": {-273.15, 1}, "kel": {-273.15, 1},
	"Rank": {-491.67, 1.8},
	"Reau": {0, 0.8},
}

// CONVERT(number, from_unit, to_unit)
func convert(_ *f.Env, a []f.Value) f.Value {
	x, err := num(a[0])
	if err != nil {
		return err
	}
	var names [2]string
	for i := range names {
		if names[i], err = f.Str(a[i+1]); err != nil {
			return err
		}
	}
	from, ok1 := lookup(names[0])
	to, ok2 := lookup(names[1])
	if !ok1 || !ok2 || from.class != to.class {
		return f.ErrNA
	}
	// scale applies the prefixes, dividing by exact powers where it can
	// so that CONVERT(1,"mm","m") is exactly 0.001.
	scale := func(x float64, dec, bin int) float64 {
		x = math.Ldexp(x, bin)
		if dec < 0 {
			return x / math.Pow10(-dec)
		}
		return x * math.Pow10(dec)
	}
	if from.class == temperature && from.name != to.name {
		ft, tt := tempScale[from.name], tempScale[to.name]
		x = scale(x, from.dec, 0)
		var y float64
		if absolute(from.name) && absolute(to.name) {
			y = x / ft.d * tt.d // without the offsets' rounding
		} else {
			y = (x+ft.offset)/ft.d*tt.d - tt.offset
		}
		return f.NumberOrErr(scale(y, -to.dec, 0))
	}
	y := x
	if from.factor != to.factor {
		y = x * from.factor / to.factor
	}
	return f.NumberOrErr(scale(y, from.dec-to.dec, from.bin-to.bin))
}

// absolute reports whether a temperature unit starts at absolute zero.
func absolute(name string) bool {
	return name == "K" || name == "kel" || name == "Rank"
}
