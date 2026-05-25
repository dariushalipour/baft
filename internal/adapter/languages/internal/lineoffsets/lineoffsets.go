package lineoffsets

import (
	"bytes"
	"sort"
)

// MakeLineOffsets precomputes the byte offset of each line start for O(1) line/col lookup.
func MakeLineOffsets(data []byte) []int {
	offsets := make([]int, 0, bytes.Count(data, []byte{'\n'})+1)
	offsets = append(offsets, 0)
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// OffsetToLineCol converts a byte offset to line/col using precomputed line offsets.
// Both line and col are 1-indexed.
func OffsetToLineCol(lineOffsets []int, data []byte, offset int) (int, int) {
	if offset > len(data) {
		offset = len(data)
	}
	if offset < 0 {
		offset = 0
	}
	idx := sort.Search(len(lineOffsets), func(i int) bool {
		return lineOffsets[i] > offset
	})
	line := idx - 1
	if line < 0 {
		line = 0
	}
	return line + 1, offset - lineOffsets[line] + 1
}
