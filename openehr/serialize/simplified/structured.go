package simplified

// REQ-053 — the STRUCTURED format and the FLAT<->STRUCTURED
// interconversion. STRUCTURED is FLAT re-nested: every non-root segment
// becomes an array (arrays throughout, even at single cardinality), leaf
// attribute suffixes become |-prefixed keys inside the array element, and the
// template-id root is a single object. The two variants share one identifier
// grammar, so interconversion needs no Web Template (spec §Conversion Between
// Formats).

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// MarshalStructured encodes comp as STRUCTURED JSON using wt.
func MarshalStructured(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	flat, err := encodeFlat(comp, wt)
	if err != nil {
		return nil, err
	}
	s, err := flatToStructured(flat)
	if err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// UnmarshalStructured decodes STRUCTURED JSON into a canonical COMPOSITION
// using wt. It restructures to FLAT and decodes that as [UnmarshalFlat] does.
//
// A member the decode does not read is ignored by default, and the value
// holds the rest of the body. [WithUnmatched] reports each such member by its
// path of members from the root, and [WithStrict] refuses the first, wrapping
// [ErrUnknownPath].
func UnmarshalStructured(data []byte, wt *webtemplate.WebTemplate, opts ...Option) (*rm.Composition, error) {
	s, err := unmarshalObject(data)
	if err != nil {
		return nil, err
	}
	var judge *webtemplate.WebTemplate
	if wt != nil && wt.Tree != nil {
		judge = wt
	}
	flat, skipped, err := structuredToFlatJudged(s, judge)
	if err != nil {
		return nil, err
	}
	if judge == nil {
		return nil, ErrNoTemplate
	}
	cfg := newDecodeConfig(opts)
	var loc locator
	if cfg.tracked() {
		if loc, err = structuredLocations(data); err != nil {
			return nil, err
		}
	}
	return decodeFlatMap(flat, wt, cfg, loc, memberKeys(skipped))
}

// FlatToStructured restructures FLAT JSON into STRUCTURED JSON (no OPT needed).
func FlatToStructured(data []byte) ([]byte, error) {
	flat, err := unmarshalObject(data)
	if err != nil {
		return nil, err
	}
	s, err := flatToStructured(flat)
	if err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// StructuredToFlat restructures STRUCTURED JSON into FLAT JSON (no OPT needed).
func StructuredToFlat(data []byte) ([]byte, error) {
	s, err := unmarshalObject(data)
	if err != nil {
		return nil, err
	}
	flatMap, err := structuredToFlat(s)
	if err != nil {
		return nil, err
	}
	return json.Marshal(flatMap)
}

// flatToStructured nests a FLAT map. The first path segment (template id) is a
// single object; every deeper segment is an array indexed by its :index. Keys
// are processed in sorted order — output must not depend on Go map iteration
// (determinism is a hard requirement) — and a key whose placement conflicts
// with another key's (scalar vs object at one slot) is an error, never a
// last-write-wins overwrite.
func flatToStructured(flat map[string]any) (map[string]any, error) {
	// `x|` spells the bare value of `x`: the two are one key here, and differing
	// values are refused, since restructuring cannot tell which one a decode
	// would read.
	flat, fold := foldEmptySuffix(flat)
	if err := fold.settle(nil); err != nil {
		return nil, err
	}
	root := make(map[string]any)
	budget := &allocBudget{limit: maxTotalNodes}
	var ctxObj map[string]any
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		val := flat[key]
		// Context is grouped under a ctx object with direct (non-arrayified)
		// values, unlike clinical data (spec §Structured format).
		if rest, ok := strings.CutPrefix(key, "ctx/"); ok {
			if ctxObj == nil {
				ctxObj = make(map[string]any)
				root["ctx"] = ctxObj
			}
			// A suffixed ctx field nests its |suffix members under the field name,
			// the same shape a clinical leaf takes — `ctx/setting|code` becomes
			// ctx.setting["|code"], not a literal "setting|code" member. ctx/setting
			// (REQ-053, 2026-08-05) is the first suffixed ctx field, so this is the
			// first time the distinction is observable; a flat piped member would
			// not round-trip through a reference-shaped STRUCTURED body.
			if base, suffix, piped := strings.Cut(rest, "|"); piped {
				leaf, isObj := ctxObj[base].(map[string]any)
				if !isObj {
					if prev, taken := ctxObj[base]; taken {
						return nil, fmt.Errorf("%w: ctx/%s is both a direct value (%#v) and a suffixed leaf (%q)",
							ErrUnknownPath, base, prev, key)
					}
					leaf = make(map[string]any)
					ctxObj[base] = leaf
				}
				leaf["|"+suffix] = val
				continue
			}
			if prev, taken := ctxObj[rest].(map[string]any); taken {
				return nil, fmt.Errorf("%w: ctx/%s is both a suffixed leaf (%#v) and a direct value (%q)",
					ErrUnknownPath, rest, prev, key)
			}
			ctxObj[rest] = val
			continue
		}
		pk, err := parseFlatKey(key)
		if err != nil {
			return nil, err
		}
		if len(pk.segs) == 0 || pk.segs[0].id == "" {
			return nil, fmt.Errorf("%w: empty root segment in %q", ErrUnknownPath, key)
		}
		rootID := pk.segs[0].id
		obj, _ := root[rootID].(map[string]any)
		if obj == nil {
			obj = make(map[string]any)
			root[rootID] = obj
		}
		rest := pk.segs[1:]
		if len(rest) == 0 {
			if pk.suffix == "" {
				// A bare value on the root segment has no STRUCTURED slot; the
				// decoder rejects the same key, so dropping it here would make
				// interconversion silently lossy.
				return nil, fmt.Errorf("%w: bare value at root segment %q", ErrUnknownPath, key)
			}
			obj["|"+pk.suffix] = val
			continue
		}
		if err := insertStructured(obj, rest, pk.suffix, val, budget); err != nil {
			return nil, fmt.Errorf("simplified: structured %q: %w", key, err)
		}
	}
	return root, nil
}

// bareStructuredKey is the STRUCTURED member a leaf's **bare** FLAT value takes
// when that leaf also carries |suffixes or nested `_` attributes, so the array
// element has to be an object: `"|"` — the `"|"+suffix` convention with the empty
// suffix the FLAT key itself spells.
//
// A leaf carrying *only* a bare value stays a bare scalar in the array, which is
// what every STRUCTURED body this codec has ever emitted looks like; the key
// appears exactly where the alternative is a scalar/object collision. Several FLAT
// leaves genuinely have that shape — a DV_TEXT beside `|formatting`, a DV_COUNT
// beside `|normal_status`, any bare leaf carrying a REQ-140 `_` family, and (the
// case that forced this) a DV_MULTIMEDIA, whose bare key is the uri and whose
// `|mediatype` + `|size` are RM-mandatory. `"|"` is not a legal FLAT suffix, so the
// inverse mapping is unambiguous without an OPT, which `|value` would not be
// (DV_ORDINAL and DV_CODED_TEXT spell a real `|value`).
const bareStructuredKey = "|"

// insertStructured places val at segs (relative to obj), growing arrays by
// :index. A bare leaf sets the array element to the scalar value, unless the slot
// already holds (or later needs) an object, in which case it takes
// [bareStructuredKey]; a suffixed leaf sets a |suffix key on the element object.
// The :index is bounded so a hostile key cannot force an unbounded allocation; two
// values competing for one slot is an error, never a last-write-wins overwrite.
func insertStructured(obj map[string]any, segs []flatSeg, suffix string, val any, budget *allocBudget) error {
	seg := segs[0]
	idx := max(seg.idx, 0)
	if idx > maxRepeatIndex {
		return fmt.Errorf("%w: :index %d on %q exceeds bound %d", ErrUnknownPath, idx, seg.id, maxRepeatIndex)
	}
	arr, _ := obj[seg.id].([]any)
	if need := idx + 1 - len(arr); need > 0 {
		if err := budget.add(need); err != nil {
			return err
		}
	}
	for len(arr) <= idx {
		arr = append(arr, nil)
	}
	obj[seg.id] = arr

	isLeaf := len(segs) == 1
	if isLeaf && suffix == "" {
		switch cur := arr[idx].(type) {
		case nil:
			arr[idx] = val
		case map[string]any:
			if _, taken := cur[bareStructuredKey]; taken {
				return fmt.Errorf("%w: conflicting values at %q index %d", ErrUnknownPath, seg.id, idx)
			}
			cur[bareStructuredKey] = val
		default:
			return fmt.Errorf("%w: conflicting values at %q index %d", ErrUnknownPath, seg.id, idx)
		}
		return nil
	}
	el, ok := arr[idx].(map[string]any)
	if !ok {
		el = make(map[string]any)
		// A scalar already at this slot is the same leaf's bare value, placed before
		// its suffixes by the sorted key order: it moves onto the object rather than
		// colliding with it.
		if arr[idx] != nil {
			el[bareStructuredKey] = arr[idx]
		}
		arr[idx] = el
	}
	if isLeaf {
		if _, taken := el["|"+suffix]; taken {
			return fmt.Errorf("%w: conflicting values at %q index %d", ErrUnknownPath, seg.id, idx)
		}
		el["|"+suffix] = val
		return nil
	}
	return insertStructured(el, segs[1:], suffix, val, budget)
}

// structuredToFlat flattens a STRUCTURED map into a FLAT map. Each array
// element takes a :index; |-prefixed keys become the FLAT |suffix. A malformed
// shape (non-object root, non-array clinical child) is an error rather than a
// silent drop (REQ-053). A null is absent (REQ-053), and the restructuring
// carries it as written: a null array element becomes a FLAT key holding null
// at its :index, and a null member a FLAT key holding null without one, which
// a decode then reads as absent wherever the key resolves. A null top-level
// member (the template root, ctx) has no FLAT key to hold it, and is absent.
func structuredToFlat(s map[string]any) (map[string]any, error) {
	out, _, err := structuredToFlatJudged(s, nil)
	return out, err
}

// structuredToFlatJudged is [structuredToFlat] for a decode against wt. At
// every level of the body a member whose name the FLAT grammar does not read
// there is unmatched content whatever the JSON shape of its value (REQ-170):
// it is set aside, not flattened, and skipped returns its member path,
// slash-joined with each array position as a segment of its own. A name the
// grammar reads keeps the shape refusals. [memberLevel] holds the rule. With a
// nil wt nothing is judged, as OPT-free conversion needs.
func structuredToFlatJudged(s map[string]any, wt *webtemplate.WebTemplate) (map[string]any, []string, error) {
	w := structuredWalker{out: make(map[string]any), wt: wt}
	for rootID, v := range s {
		if wt != nil && rootID != wt.Tree.ID && rootID != "ctx" {
			w.skipped = append(w.skipped, rootID)
			continue
		}
		if v == nil {
			// Absent (REQ-053): no FLAT key holds a root or the ctx object.
			continue
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("%w: structured root %q is not an object (%T)", ErrUnknownPath, rootID, v)
		}
		// The ctx object holds direct values (no arrays / :index), inverse of
		// the ctx grouping in flatToStructured.
		if rootID == "ctx" {
			if err := w.ctx(obj); err != nil {
				return nil, nil, err
			}
			continue
		}
		level := opaqueLevel
		if wt != nil {
			level = nodeLevel(wt.Tree, "")
		}
		if err := w.walk(rootID, rootID, level, obj); err != nil {
			return nil, nil, err
		}
	}
	return w.out, w.skipped, nil
}

// structuredWalker accumulates the FLAT entries of a STRUCTURED body and,
// against a Web Template, the member paths it set aside as unmatched.
type structuredWalker struct {
	out     map[string]any
	wt      *webtemplate.WebTemplate
	skipped []string
}

// ctxFieldNames are the ctx/ fields the decode reads, by member name: the
// fields of [ctxFields], each suffix pair under its one field. A ctx member
// is judged by its base ([memberBase]), so `"setting|code"` is the setting's.
var ctxFieldNames = func() map[string]bool {
	names := make(map[string]bool, len(ctxFields))
	for key := range ctxFields {
		name, _, _ := strings.Cut(strings.TrimPrefix(key, "ctx/"), "|")
		names[name] = true
	}
	return names
}()

// ctx flattens the fields of the ctx object. A field the decode does not read
// is set aside whatever it holds; so is a member of a field's object that is
// not a suffix, since a ctx/ field reads its suffixes and nothing else.
func (w *structuredWalker) ctx(obj map[string]any) error {
	for _, k := range slices.Sorted(maps.Keys(obj)) {
		cv := obj[k]
		if w.wt != nil && !ctxFieldNames[memberBase(k)] {
			w.skipped = append(w.skipped, "ctx/"+k)
			continue
		}
		// Inverse of the suffix nesting above: a ctx field carrying |suffix
		// members flattens back to one piped FLAT key per member.
		if leaf, nested := cv.(map[string]any); nested {
			// An empty leaf object is refused, not skipped: flattening it to
			// nothing turns an explicit `"setting": {}` into an absent field,
			// which a WithTemplate decode then completes with the
			// 238|other care default the body plainly did not ask for — the
			// same "explicit empty is not absent" rule parseCtx enforces for
			// the FLAT pair (REQ-053).
			if len(leaf) == 0 {
				return fmt.Errorf("%w: ctx/%s is an empty object; omit the field for an absent value",
					ErrUnsupportedDatatype, k)
			}
			for _, sk := range slices.Sorted(maps.Keys(leaf)) {
				if strings.HasPrefix(sk, "|") {
					w.out["ctx/"+k+sk] = leaf[sk]
					continue
				}
				if w.wt == nil {
					return fmt.Errorf("%w: ctx/%s carries member %q; a suffixed ctx leaf holds only |suffix members",
						ErrUnknownPath, k, sk)
				}
				w.skipped = append(w.skipped, "ctx/"+k+"/"+sk)
			}
			continue
		}
		w.out["ctx/"+k] = cv
	}
	return nil
}

// walk descends an object whose keys are either member names (mapping to
// arrays) or |-prefixed suffixes, accumulating FLAT entries under path.
// member is the object's member path and level what the FLAT grammar reads
// among its members ([memberLevel.judge]). An element that contributes no
// FLAT entry at all ("{}", or an object holding only empty arrays) would
// vanish from the conversion — it is an error, per the fail-loud posture —
// unless what it held was set aside as unmatched.
func (w *structuredWalker) walk(path, member string, level memberLevel, obj map[string]any) error {
	for k, v := range obj {
		if k == bareStructuredKey {
			// The leaf's bare value, spelled as a member because the element also
			// carries suffixes or nested `_` attributes (see [bareStructuredKey]).
			if _, taken := w.out[path]; taken {
				return fmt.Errorf("%w: structured: %q is spelled twice", ErrUnknownPath, path)
			}
			w.out[path] = v
			continue
		}
		if strings.HasPrefix(k, "|") {
			w.out[path+k] = v
			continue
		}
		below, read := level.judge(k)
		if !read {
			w.skipped = append(w.skipped, member+"/"+k)
			continue
		}
		if v == nil {
			// A null member is carried as written, for the decode to read as
			// absent (REQ-053).
			w.out[path+"/"+k] = nil
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%w: structured: expected an array at %q, got %T", ErrUnknownPath, path+"/"+k, v)
		}
		for i, el := range arr {
			// The position is judged before what the element holds, as the
			// FLAT decode judges a key's segments from the left (REQ-170).
			if err := below.bound.check(path, k, i); err != nil {
				return err
			}
			seg := path + "/" + k + ":" + strconv.Itoa(i)
			switch obj := el.(type) {
			case map[string]any:
				before, skippedBefore := len(w.out), len(w.skipped)
				if err := w.walk(seg, member+"/"+k+"/"+strconv.Itoa(i), below, obj); err != nil {
					return err
				}
				if len(w.out) == before && len(w.skipped) == skippedBefore {
					return fmt.Errorf("%w: structured: element at %q carries no entries", ErrUnknownPath, seg)
				}
			default:
				// A scalar, or a null element carried as written for the
				// decode to read as absent (REQ-053).
				w.out[seg] = el
			}
		}
	}
	return nil
}
