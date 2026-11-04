package aom14

import (
	"bytes"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// read_detect.go: when [ReadOperationalTemplate] reads a document a second
// time, through [tolerate]. A document the parser and canonical XML read
// alike, which most are, pays for the checks below and for no second reading.
// Each reading tolerate makes applies to a document only where the first
// decode failed in a way this file names, or where the checks below find
// what the reading changes, so the result never depends on whether the
// second reading ran.

// retryable reports whether err, the failure of the first decode, is one a
// reading of [tolerate] can lift: the text of a value, a type a slot cannot
// dispatch, which only the default mode reads but inside a C_DV_QUANTITY,
// whose assumed value every mode reads as absent where the model cannot hold
// it, or an element in the namespace the AM Release-1.4 schemas declare,
// which a strict decode refuses. A type a slot cannot dispatch covers a
// <children> of a primitive or an attribute class and an <item> of a class
// the parser reads a constraint of; the text of a value covers a real number
// canonical XML does not read and an integer bound written as a decimal.
func retryable(err error, strict bool) bool {
	switch {
	case xmlunmatched.IsPrimitiveFailure(err), errors.Is(err, canxml.ErrNamespace), failsInsideQuantity(err):
		return true
	case strict:
		return false
	}
	return isTypeFailure(err)
}

// isTypeFailure reports whether err is the failure of a type a slot cannot
// dispatch.
func isTypeFailure(err error) bool {
	return errors.Is(err, typereg.ErrMissingType) || errors.Is(err, typereg.ErrUnknownType) || errors.Is(err, typereg.ErrTypeMismatch)
}

// failsInsideQuantity reports whether err is the failure of a type a slot
// inside a C_DV_QUANTITY cannot dispatch, such as a bound of the normal range
// of its assumed value.
func failsInsideQuantity(err error) bool {
	if !isTypeFailure(err) {
		return false
	}
	for {
		de, ok := errors.AsType[*canxml.DecodeError](err)
		if !ok || de == nil {
			return false
		}
		if de.Type == "C_DV_QUANTITY" {
			inner, ok := errors.AsType[*canxml.DecodeError](de.Inner)
			return ok && inner != nil
		}
		err = de.Inner
	}
}

// needsTolerance reports whether data, which the first decode read into ot
// without a failure, may hold what [tolerate] reads otherwise than that
// decode: a C_BOOLEAN item without its true_valid or false_valid, or with an
// empty one, which the decode reads as false, or with an assumed value, which
// may be one the parser reads as none; the namespace the AM Release-1.4
// schemas declare; a bare type attribute, which the decode reads as an
// xsi:type; an empty element a number is read from, which the decode reads
// as 0; or a <definition> typed with a class it cannot hold, whose type the
// decode does not read. Unless strict, it also looks for an element in a
// namespace the decode does not read, which it skips, and for an archetype id
// on a constraint object the decode read as a C_COMPLEX_OBJECT, which it
// skips: more archetype_id elements than archetype roots that hold one. A
// check may find what tolerate then leaves as it is, which costs a second
// reading that changes nothing; none misses what it looks for. Each check
// reads data once, so that what they cost grows with the size of data.
func needsTolerance(data []byte, ot *OperationalTemplate, strict bool) bool {
	var m modelFacts
	m.walk(ot.Definition.Attributes)
	if m.booleanAssumes ||
		bytes.Contains(data, []byte(nsOpenEHRv2)) ||
		hasBareType(data) ||
		hasEmptyValue(data) ||
		typedDefinition(data) ||
		!booleansHoldFlags(data) {
		return true
	}
	if strict {
		return false
	}
	if ot.Definition.ArchetypeID.Value != "" {
		m.roots++
	}
	return foreignElements(data) || archetypeIDs(data) > m.roots
}

// modelFacts is what [needsTolerance] reads off the model the first decode
// gave.
type modelFacts struct {
	// booleanAssumes is true where a C_BOOLEAN item has an assumed value.
	booleanAssumes bool
	// roots counts the archetype roots that hold an archetype id.
	roots int
}

// walk reads the constraint objects under attrs into m.
func (m *modelFacts) walk(attrs []CAttribute) {
	for _, a := range attrs {
		var children []CObject
		switch v := a.(type) {
		case *CSingleAttribute:
			if v != nil {
				children = v.Children
			}
		case *CMultipleAttribute:
			if v != nil {
				children = v.Children
			}
		}
		for _, c := range children {
			switch v := c.(type) {
			case *CComplexObject:
				if v != nil {
					m.walk(v.Attributes)
				}
			case *CArchetypeRoot:
				if v != nil {
					if v.ArchetypeID.Value != "" {
						m.roots++
					}
					m.walk(v.Attributes)
				}
			case *CPrimitiveObject:
				if v == nil {
					continue
				}
				if b, ok := v.Item.(*CBoolean); ok && b != nil && b.AssumedValue != nil {
					m.booleanAssumes = true
				}
			}
		}
	}
}

// foreignElements reports whether data may hold an element in a namespace
// the default mode reads as canonical XML's (isForeignNamespace): whether it
// declares such a namespace as the default one, or under a prefix the name
// of an element carries, or whether the name of an element carries a prefix
// data declares nowhere, which encoding/xml reads as a namespace of that
// name. A declaration in text or a comment counts as well.
func foreignElements(data []byte) bool {
	prefixes := elementPrefixes(data)
	declared := make(map[string]bool, len(prefixes))
	foreign := false
	declarations(data, func(prefix, uri []byte) bool {
		declared[string(prefix)] = true
		if isForeignNamespace(string(uri)) && (len(prefix) == 0 || prefixes[string(prefix)]) {
			foreign = true
		}
		return !foreign
	})
	if foreign {
		return true
	}
	for p := range prefixes {
		if !declared[p] && p != "xml" {
			return true
		}
	}
	return false
}

// elementPrefixes returns the prefixes the names in the start tags of data
// carry, and in what reads as one in a comment or a CDATA section. It looks
// at each colon, far rarer than a `<`, and back over the name bytes before
// it, which no other colon's look reaches, so it reads each byte at most
// twice.
func elementPrefixes(data []byte) map[string]bool {
	prefixes := make(map[string]bool)
	for i := 0; ; {
		k := bytes.IndexByte(data[i:], ':')
		if k < 0 {
			return prefixes
		}
		j := i + k
		i = j + 1
		start := j
		for start > 0 && !endsName[data[start-1]] {
			start--
		}
		if start > 0 && start < j && data[start-1] == '<' {
			prefixes[string(data[start:j])] = true
		}
	}
}

// endsName holds the bytes that end the prefix of a name in a tag.
var endsName = func() (t [256]bool) {
	for _, c := range []byte(" \t\r\n/>:<=\"'!?") {
		t[c] = true
	}
	return t
}()

// declarations calls fn with the prefix and the namespace of each namespace
// declaration in data, an attribute named xmlns, the prefix empty, or
// xmlns:prefix, until fn returns false. A namespace written with a reference
// reads as itself, a namespace of no openEHR schema. What reads as a
// declaration in text or a comment counts as well.
func declarations(data []byte, fn func(prefix, uri []byte) bool) {
	word := []byte("xmlns")
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			return
		}
		j += i
		i = j + len(word)
		if j == 0 || !isXMLSpaceByte(data[j-1]) {
			continue
		}
		k := i
		var prefix []byte
		if k < len(data) && data[k] == ':' {
			end := bytes.IndexAny(data[k+1:], " \t\r\n/>:<=\"'")
			if end <= 0 {
				continue
			}
			prefix = data[k+1 : k+1+end]
			k += 1 + end
		}
		k = skipSpace(data, k)
		if k >= len(data) || data[k] != '=' {
			continue
		}
		k = skipSpace(data, k+1)
		if k >= len(data) || (data[k] != '"' && data[k] != '\'') {
			continue
		}
		end := bytes.IndexByte(data[k+1:], data[k])
		if end < 0 {
			return
		}
		if !fn(prefix, data[k+1:k+1+end]) {
			return
		}
	}
}

// skipSpace returns the offset of the first byte of data at or after i that
// is no XML white space.
func skipSpace(data []byte, i int) int {
	for i < len(data) && isXMLSpaceByte(data[i]) {
		i++
	}
	return i
}

// typedDefinition reports whether data may hold a <definition> typed with a
// class it cannot hold: a start tag whose local name is definition, with a
// prefix or without, that holds a type attribute, with a prefix or without,
// whose value is other than C_ARCHETYPE_ROOT, T_ARCHETYPE_ROOT and
// C_COMPLEX_OBJECT as written.
func typedDefinition(data []byte) bool {
	found := false
	startTags(data, "definition", func(tag []byte) bool {
		found = holdsOtherType(tag)
		return !found
	})
	return found
}

// holdsOtherType reports whether tag, a start tag, holds an attribute whose
// local name is type and whose value is other than C_ARCHETYPE_ROOT,
// T_ARCHETYPE_ROOT and C_COMPLEX_OBJECT as written.
func holdsOtherType(tag []byte) bool {
	word := []byte("type")
	for i := 0; ; {
		j := bytes.Index(tag[i:], word)
		if j < 0 {
			return false
		}
		j += i
		i = j + len(word)
		if j == 0 || (!isXMLSpaceByte(tag[j-1]) && tag[j-1] != ':') {
			continue
		}
		k := skipSpace(tag, i)
		if k >= len(tag) || tag[k] != '=' {
			continue
		}
		k = skipSpace(tag, k+1)
		if k >= len(tag) || (tag[k] != '"' && tag[k] != '\'') {
			continue
		}
		end := bytes.IndexByte(tag[k+1:], tag[k])
		if end < 0 {
			return true
		}
		switch string(tag[k+1 : k+1+end]) {
		case "C_ARCHETYPE_ROOT", "T_ARCHETYPE_ROOT", "C_COMPLEX_OBJECT":
		default:
			return true
		}
	}
}

// archetypeIDs counts the start tags in data whose local name is
// archetype_id, with a prefix or without. Text that reads so, in a comment
// or a CDATA section, counts as well.
func archetypeIDs(data []byte) int {
	n := 0
	startTags(data, "archetype_id", func([]byte) bool {
		n++
		return true
	})
	return n
}

// startTags calls fn with each start tag in data whose local name is local,
// with a prefix or without, from its `<` to its end (tagEnd), until fn
// returns false. Text that reads so, in a comment or a CDATA section, counts
// as well. It reads data once: it looks for the last `<` before a match, and
// where the name after it ends, in the bytes it has not looked through yet,
// and keeps both, rather than look back to them at each match.
func startTags(data []byte, local string, fn func(tag []byte) bool) {
	word := []byte(local)
	scanned := 0 // the bytes before this are looked through for a '<'
	lt := -1     // the offset of the last '<' before scanned
	brk := -1    // the offset of the first white space, '>' or '<' after lt, once read
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			return
		}
		j += i
		i = j + len(word)
		if j == 0 || i >= len(data) || !isTagEnd(data[i]) || (data[j-1] != '<' && data[j-1] != ':') {
			continue
		}
		if k := bytes.LastIndexByte(data[scanned:j], '<'); k >= 0 {
			lt, brk = scanned+k, -1
		}
		scanned = j
		switch {
		case lt == j-1:
		case lt >= 0 && data[j-1] == ':' && data[lt+1] != '/':
			// The name carries a prefix: no white space or `>` stands between
			// the `<` and the match.
			if brk < 0 {
				brk = len(data)
				if k := bytes.IndexAny(data[lt+1:], " \t\r\n><"); k >= 0 {
					brk = lt + 1 + k
				}
			}
			if brk < j {
				continue
			}
		default:
			continue
		}
		end := tagEnd(data, lt)
		if !fn(data[lt:end]) {
			return
		}
		i = max(i, end)
	}
}

// tagEnd returns the offset just past the start tag whose `<` stands at lt
// in data: past the first `>` outside a quoted attribute value. A `<` ends it
// before that, at its own offset, since no tag holds one, and so does the end
// of data.
func tagEnd(data []byte, lt int) int {
	for i := lt + 1; ; {
		k := bytes.IndexAny(data[i:], `<>"'`)
		if k < 0 {
			return len(data)
		}
		i += k
		switch data[i] {
		case '<':
			return i
		case '>':
			return i + 1
		}
		quote := `"<`
		if data[i] == '\'' {
			quote = `'<`
		}
		k = bytes.IndexAny(data[i+1:], quote)
		if k < 0 {
			return len(data)
		}
		i += 1 + k
		if data[i] == '<' {
			return i
		}
		i++
	}
}

// booleansHoldFlags reports whether each element whose start tag holds the
// word C_BOOLEAN, as the type of a C_BOOLEAN item does, holds a true_valid
// and a false_valid among the elements it holds itself, each without a
// prefix: a flag that stands deeper, or under a prefix that may be bound to
// a namespace the reading does not read, reads as none. A C_BOOLEAN anywhere
// else, such as in text, a comment or an element such an element holds,
// reads as an element without them, and so does markup the walk does not
// read, such as a document type with an internal subset. It reads data once:
// it looks for the last `<` before a match in the bytes it has not looked
// through yet, and walks the markup of the element that tag opens, rather
// than the whole document.
func booleansHoldFlags(data []byte) bool {
	word := []byte("C_BOOLEAN")
	scanned := 0 // the bytes before this are looked through for a '<'
	lt := -1     // the offset of the last '<' before scanned
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			return true
		}
		j += i
		if k := bytes.LastIndexByte(data[scanned:j], '<'); k >= 0 {
			lt = scanned + k
		}
		if lt < 0 {
			return false
		}
		end := tagEnd(data, lt)
		if end <= j || data[end-1] != '>' || data[lt+1] == '/' || data[lt+1] == '!' || data[lt+1] == '?' {
			// Not in a start tag, or in one the walk does not read.
			return false
		}
		next, ok := holdsFlags(data, lt, end)
		if !ok {
			return false
		}
		i, scanned = next, next
	}
}

// holdsFlags walks the element whose start tag spans data from lt to end,
// and reports whether it holds a true_valid and a false_valid itself, each
// without a prefix, and holds no element whose start tag holds the word
// C_BOOLEAN, and returns the offset just past it. It reports false for
// markup it does not read and for an element whose start tag closes it.
func holdsFlags(data []byte, lt, end int) (int, bool) {
	if data[end-2] == '/' {
		return 0, false
	}
	word := []byte("C_BOOLEAN")
	var seen uint8
	depth := 1
	for i := end; ; {
		k := bytes.IndexByte(data[i:], '<')
		if k < 0 {
			return 0, false
		}
		lt := i + k
		rest := data[lt+1:]
		end := -1 // the offset just past the markup, -1 where it does not end
		switch {
		case bytes.HasPrefix(rest, []byte("!--")):
			end = pastIndex(data, lt, "-->")
		case bytes.HasPrefix(rest, []byte("![CDATA[")):
			end = pastIndex(data, lt, "]]>")
		case bytes.HasPrefix(rest, []byte("?")):
			end = pastIndex(data, lt, "?>")
		case bytes.HasPrefix(rest, []byte("!")):
			// A declaration inside an element is not read.
		case bytes.HasPrefix(rest, []byte("/")):
			if k := bytes.IndexAny(rest, "<>"); k >= 0 && rest[k] == '>' {
				end = lt + 1 + k + 1
			}
			if depth--; depth == 0 && end >= 0 {
				return end, seen == seenTrueValid|seenFalseValid
			}
		default:
			end = tagEnd(data, lt)
			if data[end-1] != '>' {
				return 0, false
			}
			tag := data[lt:end]
			if bytes.Contains(tag, word) {
				return 0, false
			}
			if depth == 1 {
				switch string(tagName(tag)) {
				case "true_valid":
					seen |= seenTrueValid
				case "false_valid":
					seen |= seenFalseValid
				}
			}
			if tag[len(tag)-2] != '/' {
				depth++
			}
		}
		if end < 0 {
			return 0, false
		}
		i = end
	}
}

// pastIndex returns the offset just past the first sep in data at or after
// from, or -1 where data holds none there.
func pastIndex(data []byte, from int, sep string) int {
	k := bytes.Index(data[from:], []byte(sep))
	if k < 0 {
		return -1
	}
	return from + k + len(sep)
}

// tagName returns the name of tag, a start tag, as written, with its prefix.
func tagName(tag []byte) []byte {
	name := tag[1:]
	if end := bytes.IndexAny(name, " \t\r\n/>"); end >= 0 {
		name = name[:end]
	}
	return name
}

// isTagEnd reports whether c may follow the name of an element in its tag.
func isTagEnd(c byte) bool {
	return c == '>' || c == '/' || isXMLSpaceByte(c)
}

// hasBareType reports whether data may hold a bare type attribute: the word
// type after XML white space and before an equals sign. Text that reads so
// gives a false positive.
func hasBareType(data []byte) bool {
	word := []byte("type")
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			return false
		}
		j += i
		i = j + len(word)
		if j == 0 || !isXMLSpaceByte(data[j-1]) {
			continue
		}
		k := i
		for k < len(data) && isXMLSpaceByte(data[k]) {
			k++
		}
		if k < len(data) && data[k] == '=' {
			return true
		}
	}
}

// hasEmptyValue reports whether data holds an element that holds nothing,
// `<name/>` or `<name></name>` with attributes or none, whose local name is
// that of an element a number or a boolean is read from: list, lower, upper,
// assumed_value, true_valid or false_valid. It looks at each slash: one
// before `>` ends a start tag that closes its element, and one after `><`
// opens the end tag of an element that holds nothing. It reads data once: it
// looks for the last two `<` before a slash in the bytes it has not looked
// through yet, and keeps them and the name of the start tag each opens, once
// read, rather than look back to them at each slash.
func hasEmptyValue(data []byte) bool {
	// last is the last '<' before scanned, and from is where the bytes
	// looked through for it began; prev is the last '<' before from.
	last, prev := tagStart{at: -1}, tagStart{at: -1}
	scanned, from := 0, 0
	for i := 0; ; {
		k := bytes.IndexByte(data[i:], '/')
		if k < 0 {
			return false
		}
		j := i + k
		i = j + 1
		if k := bytes.LastIndexByte(data[scanned:j], '<'); k >= 0 {
			prev, last, from = last, tagStart{at: scanned + k}, scanned
		}
		scanned = j
		switch {
		case j+1 < len(data) && data[j+1] == '>':
			if isValueName(last.local(data)) {
				return true
			}
		case j >= 2 && data[j-1] == '<' && data[j-2] == '>':
			// The start tag is the '<' before the last: in the bytes looked
			// through with it, or else the one before them.
			before := prev
			if k := bytes.LastIndexByte(data[from:j-1], '<'); k >= 0 {
				before = tagStart{at: from + k}
			}
			if isValueName(before.local(data)) {
				return true
			}
		}
	}
}

// tagStart is a `<` of the input, at offset at, -1 for none, and the local
// name of the start tag it opens, once read.
type tagStart struct {
	at   int
	read bool
	name []byte
}

// local returns the local name of the start tag s opens in data, or nil where
// s opens none: the name after the `<`, without its prefix.
func (s *tagStart) local(data []byte) []byte {
	if s.read {
		return s.name
	}
	s.read = true
	if s.at < 0 || s.at+1 >= len(data) {
		return nil
	}
	tag := data[s.at+1:]
	if c := tag[0]; c == '/' || c == '!' || c == '?' {
		return nil
	}
	if end := bytes.IndexAny(tag, " \t\r\n/>"); end >= 0 {
		tag = tag[:end]
	}
	if _, local, ok := bytes.Cut(tag, []byte(":")); ok {
		tag = local
	}
	s.name = tag
	return tag
}

// isValueName reports whether local names an element a number or a boolean
// is read from, which canonical XML reads as 0 or false when it holds nothing.
func isValueName(local []byte) bool {
	switch string(local) {
	case "list", "lower", "upper", "assumed_value", "true_valid", "false_valid":
		return true
	}
	return false
}

// isXMLSpaceByte reports whether c is XML white space.
func isXMLSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
