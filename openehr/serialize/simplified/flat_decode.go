package simplified

// REQ-053 — FLAT decode: rebuild a canonical COMPOSITION from a FLAT map.
// The FLAT key grammar (inverse of flat_encode) is parsed here; the canonical
// RM reconstruction (walking each leaf's Web Template aqlPath, materialising
// the elided HISTORY / ITEM_TREE wrappers via rminfo, then decoding through
// canjson) builds on this parser.

import (
	"strconv"
	"strings"
)

// flatSeg is one "/"-separated FLAT path segment: a Web Template id with an
// optional zero-based instance index (idx == -1 when the segment carries no
// :index).
type flatSeg struct {
	id  string
	idx int
}

// parsedKey is a decomposed FLAT key: its path segments and the trailing
// pipe attribute suffix ("" when the key is a bare value).
type parsedKey struct {
	segs   []flatSeg
	suffix string
}

// parseFlatKey splits a FLAT key into path segments and the trailing |suffix.
// Each "/"-separated segment may carry a ":<index>" suffix; a trailing
// "|<attr>" is the leaf attribute suffix.
func parseFlatKey(key string) parsedKey {
	var suffix string
	if i := strings.LastIndex(key, "|"); i >= 0 {
		suffix = key[i+1:]
		key = key[:i]
	}
	parts := strings.Split(key, "/")
	segs := make([]flatSeg, 0, len(parts))
	for _, p := range parts {
		seg := flatSeg{id: p, idx: -1}
		if j := strings.LastIndex(p, ":"); j >= 0 {
			if n, err := strconv.Atoi(p[j+1:]); err == nil {
				seg.id = p[:j]
				seg.idx = n
			}
		}
		segs = append(segs, seg)
	}
	return parsedKey{segs: segs, suffix: suffix}
}
