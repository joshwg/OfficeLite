// Schema-less protobuf decoding: Message and its field accessors by number.

package iwork

import (
	"encoding/binary"
	"math"
)

// Protobuf wire types.
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireGroup   = 3 // start group (group markers are skipped; deprecated, unused by iWork)
	wireEnd     = 4
	wireFixed32 = 5
)

// Field is one field of an encoded protobuf message.
type Field struct {
	Num  int
	Wire int
	V    uint64 // the varint or fixed-width value
	B    []byte // the bytes of a length-delimited field
}

// Message is a protobuf message decoded to its fields in wire order. The
// schemas are not compiled in: readers ask for fields by number, using
// the field numbers of the reverse-engineered iWork .proto files. As in
// protobuf, a scalar field that occurs more than once takes its last
// value.
type Message []Field

// Decode splits b into fields. It is lenient: decoding stops at the first
// malformed field, keeping the fields before it.
func Decode(b []byte) Message {
	var m Message
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			break
		}
		b = b[n:]
		f := Field{Num: int(key >> 3), Wire: int(key & 7)}
		switch f.Wire {
		case wireVarint:
			f.V, n = binary.Uvarint(b)
			if n <= 0 {
				return m
			}
			b = b[n:]
		case wireFixed64:
			if len(b) < 8 {
				return m
			}
			f.V = binary.LittleEndian.Uint64(b)
			b = b[8:]
		case wireFixed32:
			if len(b) < 4 {
				return m
			}
			f.V = uint64(binary.LittleEndian.Uint32(b))
			b = b[4:]
		case wireBytes:
			l, n := binary.Uvarint(b)
			if n <= 0 || l > uint64(len(b)-n) {
				return m
			}
			f.B = b[n : n+int(l)]
			b = b[n+int(l):]
		case wireGroup, wireEnd:
			continue // no iWork message uses groups
		default:
			return m
		}
		m = append(m, f)
	}
	return m
}

// last returns the last occurrence of field n, and whether there is one.
func (m Message) last(n int) (Field, bool) {
	for i := len(m) - 1; i >= 0; i-- {
		if m[i].Num == n {
			return m[i], true
		}
	}
	return Field{}, false
}

// Has reports whether field n is present.
func (m Message) Has(n int) bool {
	_, ok := m.last(n)
	return ok
}

// Uint is a varint or fixed-width field as an unsigned integer (0 if
// absent).
func (m Message) Uint(n int) uint64 {
	f, _ := m.last(n)
	return f.V
}

// UintOK is Uint, also reporting whether the field is present.
func (m Message) UintOK(n int) (uint64, bool) {
	f, ok := m.last(n)
	return f.V, ok
}

// Int is an int32/int64 field (negative values are 64-bit two's
// complement varints).
func (m Message) Int(n int) int64 { return int64(m.Uint(n)) }

// Bool is a bool field.
func (m Message) Bool(n int) bool { return m.Uint(n) != 0 }

// BoolOK is Bool, also reporting whether the field is present.
func (m Message) BoolOK(n int) (bool, bool) {
	v, ok := m.UintOK(n)
	return v != 0, ok
}

// Float is a float (fixed32) field.
func (m Message) Float(n int) float64 {
	v, _ := m.FloatOK(n)
	return v
}

// FloatOK is Float, also reporting whether the field is present.
func (m Message) FloatOK(n int) (float64, bool) {
	f, ok := m.last(n)
	if !ok {
		return 0, false
	}
	if f.Wire == wireFixed64 {
		return math.Float64frombits(f.V), true
	}
	return float64(math.Float32frombits(uint32(f.V))), true
}

// Double is a double (fixed64) field; a float field is converted.
func (m Message) Double(n int) float64 { return m.Float(n) }

// Bytes is a length-delimited field's bytes.
func (m Message) Bytes(n int) []byte {
	f, _ := m.last(n)
	return f.B
}

// Str is a string field.
func (m Message) Str(n int) string { return string(m.Bytes(n)) }

// StrOK is Str, also reporting whether the field is present.
func (m Message) StrOK(n int) (string, bool) {
	f, ok := m.last(n)
	return string(f.B), ok
}

// Strs is a repeated string field.
func (m Message) Strs(n int) []string {
	var out []string
	for _, f := range m {
		if f.Num == n && f.Wire == wireBytes {
			out = append(out, string(f.B))
		}
	}
	return out
}

// Msg decodes an embedded message field (empty if absent).
func (m Message) Msg(n int) Message {
	f, ok := m.last(n)
	if !ok || f.Wire != wireBytes {
		return nil
	}
	return Decode(f.B)
}

// Msgs decodes a repeated embedded message field.
func (m Message) Msgs(n int) []Message {
	var out []Message
	for _, f := range m {
		if f.Num == n && f.Wire == wireBytes {
			out = append(out, Decode(f.B))
		}
	}
	return out
}

// Path follows a chain of embedded message fields: m.Path(1, 3) is
// m.Msg(1).Msg(3).
func (m Message) Path(nums ...int) Message {
	for _, n := range nums {
		m = m.Msg(n)
	}
	return m
}

// Uints is a repeated integer field, packed or not.
func (m Message) Uints(n int) []uint64 {
	var out []uint64
	for _, f := range m {
		if f.Num != n {
			continue
		}
		if f.Wire != wireBytes {
			out = append(out, f.V)
			continue
		}
		for b := f.B; len(b) > 0; {
			v, k := binary.Uvarint(b)
			if k <= 0 {
				break
			}
			out = append(out, v)
			b = b[k:]
		}
	}
	return out
}

// Floats is a repeated float field, packed or not.
func (m Message) Floats(n int) []float64 {
	var out []float64
	for _, f := range m {
		if f.Num != n {
			continue
		}
		if f.Wire != wireBytes {
			out = append(out, float64(math.Float32frombits(uint32(f.V))))
			continue
		}
		for b := f.B; len(b) >= 4; b = b[4:] {
			out = append(out, float64(math.Float32frombits(binary.LittleEndian.Uint32(b))))
		}
	}
	return out
}

// Ref is a TSP.Reference field's object identifier (0 if absent).
func (m Message) Ref(n int) uint64 { return m.Msg(n).Uint(1) }

// Refs is a repeated TSP.Reference field's identifiers.
func (m Message) Refs(n int) []uint64 {
	var out []uint64
	for _, r := range m.Msgs(n) {
		out = append(out, r.Uint(1))
	}
	return out
}
