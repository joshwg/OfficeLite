package ole

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf16"
)

// Stream is a named stream to write at the root of a compound file.
type Stream struct {
	Name string // e.g. "Workbook", "WordDocument", "\x05SummaryInformation"
	Data []byte
}

const (
	sectorSize     = 512
	miniSectorSize = 64
	miniCutoff     = 4096
	entriesPerDir  = sectorSize / 128
	idsPerSector   = sectorSize / 4

	freeSect   = 0xFFFFFFFF
	endOfChain = 0xFFFFFFFE
	fatSect    = 0xFFFFFFFD
	difSect    = 0xFFFFFFFC
	noStream   = 0xFFFFFFFF
)

// Write writes a version 3 compound file (512-byte sectors) holding the
// given streams at its root. Streams under 4096 bytes go in the mini
// stream, as the format requires.
func Write(w io.Writer, streams []Stream) error {
	seen := map[string]bool{}
	for _, s := range streams {
		if n := len(utf16.Encode([]rune(s.Name))); n == 0 || n > 31 {
			return fmt.Errorf("ole: bad stream name %q", s.Name)
		}
		key := strings.ToUpper(s.Name)
		if seen[key] {
			return fmt.Errorf("ole: duplicate stream %q", s.Name)
		}
		seen[key] = true
	}

	var fat []uint32
	// alloc reserves a chain of n sectors and returns its first sector.
	alloc := func(n int) uint32 {
		if n == 0 {
			return endOfChain
		}
		start := uint32(len(fat))
		for i := 0; i < n; i++ {
			fat = append(fat, uint32(len(fat)+1))
		}
		fat[len(fat)-1] = endOfChain
		return start
	}
	sectors := func(n int, size int) int { return (n + size - 1) / size }

	// Big streams first, then the mini stream, mini FAT and directory.
	starts := make([]uint32, len(streams))
	var mini []byte
	var miniFAT []uint32
	var body []byte
	for i, s := range streams {
		if len(s.Data) >= miniCutoff {
			starts[i] = alloc(sectors(len(s.Data), sectorSize))
			body = append(body, s.Data...)
			body = append(body, make([]byte, pad(len(s.Data), sectorSize))...)
		}
	}
	for i, s := range streams {
		if len(s.Data) < miniCutoff {
			if len(s.Data) == 0 {
				starts[i] = endOfChain
				continue
			}
			n := sectors(len(s.Data), miniSectorSize)
			starts[i] = uint32(len(miniFAT))
			for j := 0; j < n; j++ {
				miniFAT = append(miniFAT, uint32(len(miniFAT)+1))
			}
			miniFAT[len(miniFAT)-1] = endOfChain
			mini = append(mini, s.Data...)
			mini = append(mini, make([]byte, pad(len(s.Data), miniSectorSize))...)
		}
	}
	miniStart := alloc(sectors(len(mini), sectorSize))
	body = append(body, mini...)
	body = append(body, make([]byte, pad(len(mini), sectorSize))...)

	miniFATStart := uint32(endOfChain)
	if len(miniFAT) > 0 {
		miniFATStart = alloc(sectors(len(miniFAT)*4, sectorSize))
		buf := make([]byte, 0, sectors(len(miniFAT)*4, sectorSize)*sectorSize)
		for _, v := range miniFAT {
			buf = binary.LittleEndian.AppendUint32(buf, v)
		}
		for len(buf)%sectorSize != 0 {
			buf = binary.LittleEndian.AppendUint32(buf, freeSect)
		}
		body = append(body, buf...)
	}

	dir := directory(streams, starts, miniStart, len(mini))
	dirStart := alloc(len(dir) / sectorSize)
	body = append(body, dir...)

	// FAT and DIFAT sectors describe themselves, so size them together.
	used := len(fat)
	numFAT, numDIFAT := 0, 0
	for {
		total := used + numFAT + numDIFAT
		nf := sectors(total, idsPerSector)
		nd := 0
		if nf > 109 {
			nd = sectors(nf-109, idsPerSector-1)
		}
		if nf == numFAT && nd == numDIFAT {
			break
		}
		numFAT, numDIFAT = nf, nd
	}
	fatSectors := make([]uint32, numFAT)
	for i := range fatSectors {
		fatSectors[i] = uint32(len(fat))
		fat = append(fat, fatSect)
	}
	difatStart := uint32(endOfChain)
	difatSectors := make([]uint32, numDIFAT)
	for i := range difatSectors {
		difatSectors[i] = uint32(len(fat))
		fat = append(fat, difSect)
	}
	if numDIFAT > 0 {
		difatStart = difatSectors[0]
	}
	for len(fat)%idsPerSector != 0 {
		fat = append(fat, freeSect)
	}

	hdr := make([]byte, sectorSize)
	copy(hdr, Magic)
	le := binary.LittleEndian
	le.PutUint16(hdr[0x18:], 0x003E)
	le.PutUint16(hdr[0x1A:], 3)
	le.PutUint16(hdr[0x1C:], 0xFFFE)
	le.PutUint16(hdr[0x1E:], 9)
	le.PutUint16(hdr[0x20:], 6)
	le.PutUint32(hdr[0x2C:], uint32(numFAT))
	le.PutUint32(hdr[0x30:], dirStart)
	le.PutUint32(hdr[0x38:], miniCutoff)
	le.PutUint32(hdr[0x3C:], miniFATStart)
	le.PutUint32(hdr[0x40:], uint32(sectors(len(miniFAT)*4, sectorSize)))
	le.PutUint32(hdr[0x44:], difatStart)
	le.PutUint32(hdr[0x48:], uint32(numDIFAT))
	for i := 0; i < 109; i++ {
		v := uint32(freeSect)
		if i < numFAT {
			v = fatSectors[i]
		}
		le.PutUint32(hdr[0x4C+4*i:], v)
	}

	out := make([]byte, 0, len(hdr)+len(body)+len(fat)*4+numDIFAT*sectorSize)
	out = append(out, hdr...)
	out = append(out, body...)
	for _, v := range fat {
		out = le.AppendUint32(out, v)
	}
	rest := fatSectors[min(109, numFAT):]
	for i := range difatSectors {
		for j := 0; j < idsPerSector-1; j++ {
			v := uint32(freeSect)
			if j < len(rest) {
				v = rest[j]
			}
			out = le.AppendUint32(out, v)
		}
		rest = rest[min(len(rest), idsPerSector-1):]
		next := uint32(endOfChain)
		if i+1 < len(difatSectors) {
			next = difatSectors[i+1]
		}
		out = le.AppendUint32(out, next)
	}
	_, err := w.Write(out)
	return err
}

func pad(n, size int) int { return (size - n%size) % size }

// directory builds the directory stream: the root entry, then one entry
// per stream, linked as a binary search tree in the format's name order
// (shorter names first, then by upper-cased name).
func directory(streams []Stream, starts []uint32, miniStart uint32, miniLen int) []byte {
	order := make([]int, len(streams))
	for i := range order {
		order[i] = i
	}
	less := func(a, b string) bool {
		ua, ub := utf16.Encode([]rune(strings.ToUpper(a))), utf16.Encode([]rune(strings.ToUpper(b)))
		if len(ua) != len(ub) {
			return len(ua) < len(ub)
		}
		for i := range ua {
			if ua[i] != ub[i] {
				return ua[i] < ub[i]
			}
		}
		return false
	}
	sort.Slice(order, func(i, j int) bool { return less(streams[order[i]].Name, streams[order[j]].Name) })

	n := len(streams) + 1
	left := make([]uint32, n)
	right := make([]uint32, n)
	for i := range left {
		left[i], right[i] = noStream, noStream
	}
	// build links the sorted entries lo..hi into a balanced tree and
	// returns its root's directory id (entry ids are stream index + 1).
	var build func(lo, hi int) uint32
	build = func(lo, hi int) uint32 {
		if lo > hi {
			return noStream
		}
		mid := (lo + hi) / 2
		id := uint32(order[mid] + 1)
		left[id] = build(lo, mid-1)
		right[id] = build(mid+1, hi)
		return id
	}
	rootChild := build(0, len(order)-1)

	entries := make([]byte, 0, ((n+entriesPerDir-1)/entriesPerDir)*sectorSize)
	entry := func(name string, typ byte, l, r, child, start uint32, size uint64) {
		e := make([]byte, 128)
		u := utf16.Encode([]rune(name))
		for i, c := range u {
			binary.LittleEndian.PutUint16(e[2*i:], c)
		}
		binary.LittleEndian.PutUint16(e[0x40:], uint16(2*(len(u)+1)))
		e[0x42] = typ
		e[0x43] = 1 // black
		binary.LittleEndian.PutUint32(e[0x44:], l)
		binary.LittleEndian.PutUint32(e[0x48:], r)
		binary.LittleEndian.PutUint32(e[0x4C:], child)
		binary.LittleEndian.PutUint32(e[0x74:], start)
		binary.LittleEndian.PutUint64(e[0x78:], size)
		entries = append(entries, e...)
	}
	rootStart := miniStart
	if miniLen == 0 {
		rootStart = endOfChain
	}
	entry("Root Entry", 5, noStream, noStream, rootChild, rootStart, uint64(miniLen))
	for i, s := range streams {
		id := uint32(i + 1)
		entry(s.Name, 2, left[id], right[id], noStream, starts[i], uint64(len(s.Data)))
	}
	for len(entries)%sectorSize != 0 {
		e := make([]byte, 128)
		binary.LittleEndian.PutUint32(e[0x44:], noStream)
		binary.LittleEndian.PutUint32(e[0x48:], noStream)
		binary.LittleEndian.PutUint32(e[0x4C:], noStream)
		entries = append(entries, e...)
	}
	return entries
}
