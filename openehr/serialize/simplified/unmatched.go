package simplified

// Unmatched content on decode (REQ-170). A FLAT key is unmatched when the
// decode does not read it: its path reaches no Web Template node, an
// underscore segment names no RM attribute this codec carries at that node, a
// suffix lies outside its datatype's (or its family's) suffix set, or a ctx/
// field is one this codec does not read. By default the decode ignores such a
// key; a caller can have each one reported by its path, or the first refused.
//
// How it works. The decode sites that meet a key they do not read hand it to
// the decode's [unmatchedSink] and carry on without it. Whether a key is
// unmatched depends at most on the other spellings of its own leaf (a bare
// value is unmatched beside a suffix its datatype reads, and malformed alone),
// never on what the decode makes of the rest of the body, so the value the
// default decode must return is the decode of the body without those keys. A
// pass that met unmatched keys therefore decodes again, from the body with
// them removed: structural checks (a dense :index sequence, one instance of a
// single-valued segment) then see only the keys the decode reads, exactly as
// they would for the body without the unmatched ones. A pass that meets none
// is the result. Each repeat removes at least one key, so the loop ends.
//
// A STRUCTURED member whose name resolves nothing is set aside before the
// body becomes a FLAT map ([structuredToFlatJudged]), whatever it holds, and
// joins the keys the passes set aside.
//
// Document order is recovered only when an option asks for it: the FLAT
// object's keys are read in order, and a STRUCTURED body's members are walked
// in order to map each FLAT key the interconversion produces, and each member
// set aside, back to its member path.

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// unmatchedSink collects the unmatched keys one decode pass meets, as written
// in the FLAT body, each once. A nil sink collects nothing; every method is
// safe on it.
type unmatchedSink struct {
	keys []string
	seen map[string]bool
	// spellings maps a key the decode works on to every key as written that
	// it stands for, where that is not just itself: [foldEmptySuffix] reads
	// `x|` as `x`, so an unmatched `x` reports each spelling the body carried.
	spellings map[string][]string
}

// add records key as unmatched.
func (s *unmatchedSink) add(key string) {
	if s == nil {
		return
	}
	if written, ok := s.spellings[key]; ok {
		for _, w := range written {
			s.addWritten(w)
		}
		return
	}
	s.addWritten(key)
}

// addWritten records one key as written in the body.
func (s *unmatchedSink) addWritten(key string) {
	if s.seen[key] {
		return
	}
	if s.seen == nil {
		s.seen = make(map[string]bool)
	}
	s.seen[key] = true
	s.keys = append(s.keys, key)
}

// reported reports whether key, as written, went to the sink.
func (s *unmatchedSink) reported(key string) bool {
	return s != nil && s.seen[key]
}

// errNothingMatched is how a group decoder says that every key of its group
// was unmatched: there is nothing to place, and the caller skips the group as
// if the body did not carry it. It never leaves the package.
var errNothingMatched = errors.New("simplified: every key of the group is unmatched")

// decodeFlatIgnoring decodes flat as the default decode must: without its
// unmatched keys. It returns the canonical-JSON composition of the last pass,
// that pass's error, and every unmatched key met, in the order met.
//
// flat is never modified, and no pass writes into another's input: decode
// places a composite value (a `|raw` fragment, say) by reference and may then
// decorate it, so each pass reads its own deep copy of every composite value
// ([passInput]).
//
// stopEarly ends the loop after a first pass that found unmatched keys and
// read the whole body without failing: every unmatched key is then known, and
// a strict decode refuses whatever the next pass would make of the rest.
func decodeFlatIgnoring(flat map[string]any, wt *webtemplate.WebTemplate, names map[string]string, stopEarly bool) (map[string]any, []string, error) {
	var unmatched []string
	body := flat
	for {
		sink := &unmatchedSink{}
		compJSON, err := decodeFlat(passInput(body), wt, names, sink)
		if len(sink.keys) == 0 || (stopEarly && err == nil) {
			return compJSON, append(unmatched, sink.keys...), err
		}
		if len(unmatched) == 0 {
			body = maps.Clone(flat)
		}
		removed := 0
		for _, k := range sink.keys {
			if _, ok := body[k]; ok {
				delete(body, k)
				removed++
			}
			unmatched = append(unmatched, k)
		}
		if removed == 0 {
			// Every key reported is already gone, so a further pass would meet
			// the same body. Defensive: a reported key is always one the body
			// carries as written.
			return compJSON, unmatched, err
		}
	}
}

// passInput returns the body one decode pass reads: body itself when it holds
// no composite value, which decode cannot write into, and otherwise a copy
// whose objects and arrays are deep copies, so the pass cannot reach body's.
func passInput(body map[string]any) map[string]any {
	composite := false
	for _, v := range body {
		switch v.(type) {
		case map[string]any, []any:
			composite = true
		}
		if composite {
			break
		}
	}
	if !composite {
		return body
	}
	out := make(map[string]any, len(body))
	for k, v := range body {
		out[k] = deepCopyJSON(v)
	}
	return out
}

// deepCopyJSON copies a decoded JSON value: objects and arrays are copied all
// the way down, scalars are shared.
func deepCopyJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = deepCopyJSON(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = deepCopyJSON(e)
		}
		return out
	}
	return v
}

// keyLocation is where a FLAT key sits in the input: its position among the
// input's items and the path [WithUnmatched] reports for it.
type keyLocation struct {
	pos  int
	path string
}

// locator maps a FLAT key to its location in the input. A STRUCTURED member
// set aside before the FLAT map is made is located under its [memberKeys] key.
type locator map[string]keyLocation

// memberKeyPrefix opens the locator key of a STRUCTURED member path. No FLAT
// key starts with it, so the two never meet.
const memberKeyPrefix = "\x00"

// memberKeys returns the locator keys of STRUCTURED member paths.
func memberKeys(paths []string) []string {
	keys := make([]string, len(paths))
	for i, p := range paths {
		keys[i] = memberKeyPrefix + p
	}
	return keys
}

// order sorts keys into the order the input gives them and returns their
// paths. A key the locator does not know goes after the known ones, sorted,
// under its own spelling.
func (l locator) order(keys []string) []string {
	sorted := slices.Clone(keys)
	slices.SortStableFunc(sorted, func(a, b string) int {
		la, okA := l[a]
		lb, okB := l[b]
		switch {
		case okA && okB:
			return cmp.Compare(la.pos, lb.pos)
		case okA:
			return -1
		case okB:
			return 1
		default:
			return strings.Compare(a, b)
		}
	})
	paths := make([]string, 0, len(sorted))
	for _, k := range slices.Compact(sorted) {
		if loc, ok := l[k]; ok {
			paths = append(paths, loc.path)
			continue
		}
		paths = append(paths, strings.TrimPrefix(k, memberKeyPrefix))
	}
	return paths
}

// flatLocations reads the keys of a FLAT object in the order written. Each
// key's path is the key itself. A key written twice keeps its first position.
func flatLocations(data []byte) (locator, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	loc := make(locator)
	for pos := 0; dec.More(); pos++ {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("simplified: FLAT object key is a %T", tok)
		}
		if err := dec.Decode(new(json.RawMessage)); err != nil {
			return nil, err
		}
		if _, seen := loc[key]; !seen {
			loc[key] = keyLocation{pos: pos, path: key}
		}
	}
	return loc, nil
}

// orderedValue is a JSON value that keeps the order of an object's members:
// an object (isObject, members), an array (isArray, elems), or anything else,
// null included, which carries nothing more.
type orderedValue struct {
	isObject bool
	isArray  bool
	members  []orderedMember
	elems    []orderedValue
}

// orderedMember is one member of an object, in document order.
type orderedMember struct {
	name  string
	value orderedValue
}

// readOrdered reads the next JSON value of dec, keeping member order.
func readOrdered(dec *json.Decoder) (orderedValue, error) {
	tok, err := dec.Token()
	if err != nil {
		return orderedValue{}, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return orderedValue{}, nil
	}
	switch delim {
	case '{':
		v := orderedValue{isObject: true}
		for dec.More() {
			nameTok, err := dec.Token()
			if err != nil {
				return orderedValue{}, err
			}
			name, ok := nameTok.(string)
			if !ok {
				return orderedValue{}, fmt.Errorf("simplified: STRUCTURED member name is a %T", nameTok)
			}
			member, err := readOrdered(dec)
			if err != nil {
				return orderedValue{}, err
			}
			v.members = append(v.members, orderedMember{name: name, value: member})
		}
		_, err := dec.Token()
		return v, err
	case '[':
		v := orderedValue{isArray: true}
		for dec.More() {
			el, err := readOrdered(dec)
			if err != nil {
				return orderedValue{}, err
			}
			v.elems = append(v.elems, el)
		}
		_, err := dec.Token()
		return v, err
	default:
		return orderedValue{}, fmt.Errorf("simplified: unexpected %v in STRUCTURED body", delim)
	}
}

// structuredLocations walks a STRUCTURED body in document order and returns,
// for each FLAT key [structuredToFlat] makes of it, its position and its
// member path. The walk mirrors that conversion: the ctx object's fields, and
// under every other root member the child arrays, each element taking a
// `:index` in FLAT and its position as a segment in the member path. Every
// member that is not a suffix, the top-level ones and the ctx fields
// included, is located too, under its [memberKeys] key and ahead of what it
// holds, for a member [structuredToFlatJudged] sets aside.
func structuredLocations(data []byte) (locator, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	root, err := readOrdered(dec)
	if err != nil {
		return nil, err
	}
	loc := make(locator)
	pos := 0
	put := func(flatKey, path string) {
		if _, seen := loc[flatKey]; !seen {
			loc[flatKey] = keyLocation{pos: pos, path: path}
		}
		pos++
	}
	var walk func(flatPath, memberPath string, obj orderedValue)
	walk = func(flatPath, memberPath string, obj orderedValue) {
		for _, m := range obj.members {
			if m.name != bareStructuredKey && !strings.HasPrefix(m.name, "|") {
				put(memberKeyPrefix+memberPath+"/"+m.name, memberPath+"/"+m.name)
			}
			switch {
			case m.name == bareStructuredKey:
				put(flatPath, memberPath+"/"+m.name)
			case strings.HasPrefix(m.name, "|"):
				put(flatPath+m.name, memberPath+"/"+m.name)
			case m.value.isArray:
				for i, el := range m.value.elems {
					seg := flatPath + "/" + m.name + ":" + strconv.Itoa(i)
					elPath := memberPath + "/" + m.name + "/" + strconv.Itoa(i)
					if el.isObject {
						walk(seg, elPath, el)
						continue
					}
					put(seg, elPath)
				}
			}
		}
	}
	for _, m := range root.members {
		put(memberKeyPrefix+m.name, m.name)
		if !m.value.isObject {
			continue
		}
		if m.name == "ctx" {
			for _, field := range m.value.members {
				put(memberKeyPrefix+"ctx/"+field.name, "ctx/"+field.name)
				if field.value.isObject {
					for _, sfx := range field.value.members {
						put("ctx/"+field.name+sfx.name, "ctx/"+field.name+"/"+sfx.name)
					}
					continue
				}
				put("ctx/"+field.name, "ctx/"+field.name)
			}
			continue
		}
		walk(m.name, m.name, m.value)
	}
	return loc, nil
}

// unmatchedError is the refusal of an unmatched item under [WithStrict]. It
// names the item by its path and carries nothing else from the input, so no
// value reaches the message. It wraps [ErrUnknownPath].
type unmatchedError struct {
	path string
}

// Error names the item's path, quoted so that it reads as one token.
func (e *unmatchedError) Error() string {
	return ErrUnknownPath.Error() + ": " + strconv.Quote(e.path) + " is unmatched content"
}

// Unwrap returns [ErrUnknownPath].
func (e *unmatchedError) Unwrap() error { return ErrUnknownPath }

// applyUnmatchedPolicy reports or refuses the unmatched keys of a decode that
// has otherwise read its input, as cfg asks. It returns the strict refusal, or
// nil. loc gives each key's place in the input; it is nil when cfg tracks
// nothing.
func applyUnmatchedPolicy(cfg decodeConfig, loc locator, unmatched []string) error {
	if len(unmatched) == 0 || !cfg.tracked() {
		return nil
	}
	paths := loc.order(unmatched)
	if cfg.strict {
		if cfg.unmatched != nil {
			cfg.unmatched(paths[0])
		}
		return &unmatchedError{path: paths[0]}
	}
	for _, p := range paths {
		cfg.unmatched(p)
	}
	return nil
}

// --- suffix sets ----------------------------------------------------------

// leafSuffixMatched reports whether suffix ("" for a bare value) is one the
// decode reads at a value of rmType. judged is false for a datatype this codec
// has no suffix set for, where the caller decides.
//
// The set is the datatype's own (allowedSuffixes), plus the `|raw` carrier
// every mapped datatype takes. A DV_TEXT position also reads the DV_CODED_TEXT
// suffixes, which select the coded subtype (REQ-053); a DV_CODED_TEXT position
// reads `|other`. A STRING leaf reads its bare value and nothing else.
func leafSuffixMatched(rmType, suffix string) (matched, judged bool) {
	if rmType == stringLeafType {
		return suffix == "", true
	}
	allowed, known := allowedSuffixes[rmType]
	if !known {
		return false, false
	}
	switch {
	case allowed[suffix], suffix == "raw":
		return true, true
	case rmType == "DV_TEXT":
		return allowedSuffixes["DV_CODED_TEXT"][suffix], true
	case rmType == "DV_CODED_TEXT":
		return suffix == "other", true
	}
	return false, true
}

// matchedSuffixes returns sfx without the suffixes a value of rmType does not
// read, handing the key of each one dropped to sink. keyOf builds the FLAT key
// a suffix arrived under. For a datatype with no suffix set it returns sfx as
// it is. The map returned is sfx itself when nothing was dropped.
//
// A bare value at a datatype with no bare spelling is unmatched beside a
// suffix the datatype reads, as DV_PROPORTION's derived magnitude beside its
// numerator is. Alone it is a value missing every required suffix, malformed
// content (REQ-170): it is kept, so the datatype's own decoding refuses it.
func matchedSuffixes(rmType string, sfx map[string]any, keyOf func(suffix string) string, sink *unmatchedSink) map[string]any {
	var drop []string
	for _, suffix := range slices.Sorted(maps.Keys(sfx)) {
		matched, judged := leafSuffixMatched(rmType, suffix)
		if !judged {
			return sfx
		}
		if !matched {
			drop = append(drop, suffix)
		}
	}
	if len(drop) == 0 {
		return sfx
	}
	if _, bare := sfx[""]; bare && len(drop) == len(sfx) {
		drop = slices.DeleteFunc(drop, func(suffix string) bool { return suffix == "" })
	}
	out := maps.Clone(sfx)
	for _, suffix := range drop {
		delete(out, suffix)
		sink.add(keyOf(suffix))
	}
	return out
}
