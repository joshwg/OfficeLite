package main

import (
	"bytes"
	"encoding/binary"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	langNeutral = 0
	// iconGroupID is the resource Gio's window loads (app/os_windows.go
	// iconID) and, being the first group, the one Explorer shows.
	iconGroupID = 1
)

type resource struct {
	typ, id uint32
	data    []byte
}

// resourceObject returns a COFF object for amd64 holding a .rsrc section
// with the icon, which the Go linker merges into the executable's
// resources when it sits in the main package as a .syso file.
func resourceObject(images []iconImage) ([]byte, error) {
	var res []resource
	for i, img := range images {
		res = append(res, resource{rtIcon, uint32(i + 1), img.data})
	}
	res = append(res, resource{rtGroupIcon, iconGroupID, groupIcon(images)})
	section, relocs := resourceSection(res)

	le := binary.LittleEndian
	var b bytes.Buffer
	const fileHeader, sectionHeader, relocSize = 20, 40, 10
	rawData := fileHeader + sectionHeader
	relocAt := rawData + len(section)
	symAt := relocAt + relocSize*len(relocs)

	// IMAGE_FILE_HEADER
	b.Write(le.AppendUint16(nil, 0x8664)) // AMD64
	b.Write(le.AppendUint16(nil, 1))      // sections
	b.Write(le.AppendUint32(nil, 0))      // timestamp
	b.Write(le.AppendUint32(nil, uint32(symAt)))
	b.Write(le.AppendUint32(nil, 1)) // symbols
	b.Write(le.AppendUint16(nil, 0)) // optional header size
	b.Write(le.AppendUint16(nil, 0)) // characteristics

	// IMAGE_SECTION_HEADER
	b.WriteString(".rsrc\x00\x00\x00")
	b.Write(le.AppendUint32(nil, 0)) // virtual size
	b.Write(le.AppendUint32(nil, 0)) // virtual address
	b.Write(le.AppendUint32(nil, uint32(len(section))))
	b.Write(le.AppendUint32(nil, uint32(rawData)))
	b.Write(le.AppendUint32(nil, uint32(relocAt)))
	b.Write(le.AppendUint32(nil, 0)) // line numbers
	b.Write(le.AppendUint16(nil, uint16(len(relocs))))
	b.Write(le.AppendUint16(nil, 0))
	b.Write(le.AppendUint32(nil, 0x40000040)) // initialized data, readable

	b.Write(section)
	// Each data entry's address is relative to the section start; the
	// linker turns it into an RVA (IMAGE_REL_AMD64_ADDR32NB against the
	// section symbol).
	for _, at := range relocs {
		b.Write(le.AppendUint32(nil, uint32(at)))
		b.Write(le.AppendUint32(nil, 0)) // symbol 0
		b.Write(le.AppendUint16(nil, 3))
	}
	// The section symbol, then an empty string table.
	b.WriteString(".rsrc\x00\x00\x00")
	b.Write(le.AppendUint32(nil, 0)) // value
	b.Write(le.AppendUint16(nil, 1)) // section number
	b.Write(le.AppendUint16(nil, 0)) // type
	b.WriteByte(3)                   // IMAGE_SYM_CLASS_STATIC
	b.WriteByte(0)                   // aux symbols
	b.Write(le.AppendUint32(nil, 4))
	return b.Bytes(), nil
}

// resourceSection lays out the resource directory (type → id → language
// → data entry) followed by the data, and returns the offsets of the data
// entries' address fields, which need relocating. res is sorted by type
// and then id, as the directory requires.
func resourceSection(res []resource) ([]byte, []int) {
	le := binary.LittleEndian
	var types []uint32
	byType := map[uint32][]resource{}
	for _, r := range res {
		if len(byType[r.typ]) == 0 {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], r)
	}

	dirSize := func(n int) int { return 16 + 8*n }
	// Offsets: root, then each type's id directory, then a language
	// directory per resource, then the data entries, then the data.
	off := dirSize(len(types))
	typeDir := map[uint32]int{}
	for _, t := range types {
		typeDir[t] = off
		off += dirSize(len(byType[t]))
	}
	langDir := make([]int, len(res))
	for i := range res {
		langDir[i] = off
		off += dirSize(1)
	}
	entry := make([]int, len(res))
	for i := range res {
		entry[i] = off
		off += 16
	}
	dataAt := make([]int, len(res))
	for i, r := range res {
		off = (off + 7) &^ 7
		dataAt[i] = off
		off += len(r.data)
	}

	out := make([]byte, off)
	dir := func(at, n int) int {
		le.PutUint16(out[at+14:], uint16(n)) // id entries
		return at + 16
	}
	const subdir = 0x80000000
	p := dir(0, len(types))
	for _, t := range types {
		le.PutUint32(out[p:], t)
		le.PutUint32(out[p+4:], uint32(typeDir[t])|subdir)
		p += 8
	}
	i := 0
	for _, t := range types {
		p := dir(typeDir[t], len(byType[t]))
		for _, r := range byType[t] {
			le.PutUint32(out[p:], r.id)
			le.PutUint32(out[p+4:], uint32(langDir[i])|subdir)
			p += 8
			q := dir(langDir[i], 1)
			le.PutUint32(out[q:], langNeutral)
			le.PutUint32(out[q+4:], uint32(entry[i]))
			le.PutUint32(out[entry[i]:], uint32(dataAt[i]))
			le.PutUint32(out[entry[i]+4:], uint32(len(r.data)))
			copy(out[dataAt[i]:], r.data)
			i++
		}
	}
	return out, entry
}
