package simplified

// REQ-053 — STRUCTURED (structSDT) format and the FLAT<->STRUCTURED
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

// MarshalStructured encodes comp as STRUCTURED JSON using wt (REQ-053).
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
// using wt (REQ-053). It restructures to FLAT and delegates to UnmarshalFlat.
func UnmarshalStructured(data []byte, wt *webtemplate.WebTemplate, opts ...Option) (*rm.Composition, error) {
	s, err := unmarshalObject(data)
	if err != nil {
		return nil, err
	}
	flatMap, err := structuredToFlat(s)
	if err != nil {
		return nil, err
	}
	flat, err := json.Marshal(flatMap)
	if err != nil {
		return nil, err
	}
	return UnmarshalFlat(flat, wt, opts...)
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

// insertStructured places val at segs (relative to obj), growing arrays by
// :index. A bare leaf sets the array element to the scalar value; a suffixed
// leaf sets a |suffix key on the element object. The :index is bounded so a
// hostile key cannot force an unbounded allocation; a scalar/object collision
// at one slot is an error.
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
		if arr[idx] != nil {
			return fmt.Errorf("%w: conflicting values at %q index %d", ErrUnknownPath, seg.id, idx)
		}
		arr[idx] = val
		return nil
	}
	el, ok := arr[idx].(map[string]any)
	if !ok {
		if arr[idx] != nil {
			return fmt.Errorf("%w: conflicting scalar and object at %q index %d", ErrUnknownPath, seg.id, idx)
		}
		el = make(map[string]any)
		arr[idx] = el
	}
	if isLeaf {
		el["|"+suffix] = val
		return nil
	}
	return insertStructured(el, segs[1:], suffix, val, budget)
}

// structuredToFlat flattens a STRUCTURED map into a FLAT map. Each array
// element takes a :index; |-prefixed keys become the FLAT |suffix. A malformed
// shape (non-object root, non-array clinical child, null array hole) is an error
// rather than a silent drop (REQ-053).
func structuredToFlat(s map[string]any) (map[string]any, error) {
	out := make(map[string]any)
	for rootID, v := range s {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("simplified: structured: root %q is not an object (%T)", rootID, v)
		}
		// The ctx object holds direct values (no arrays / :index), inverse of
		// the ctx grouping in flatToStructured.
		if rootID == "ctx" {
			for k, cv := range obj {
				out["ctx/"+k] = cv
			}
			continue
		}
		if err := structWalk(out, rootID, obj); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// structWalk descends an object whose keys are either child ids (mapping to
// arrays) or |-prefixed leaf suffixes, accumulating FLAT entries under path. An
// element that contributes no FLAT entry at all ("{}", or an object holding
// only empty arrays) would vanish from the conversion — it is an error, per the
// fail-loud posture.
func structWalk(out map[string]any, path string, obj map[string]any) error {
	for k, v := range obj {
		if strings.HasPrefix(k, "|") {
			out[path+k] = v
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("simplified: structured: expected an array at %q, got %T", path+"/"+k, v)
		}
		for i, el := range arr {
			seg := path + "/" + k + ":" + strconv.Itoa(i)
			switch child := el.(type) {
			case map[string]any:
				before := len(out)
				if err := structWalk(out, seg, child); err != nil {
					return err
				}
				if len(out) == before {
					return fmt.Errorf("simplified: structured: element at %q carries no entries", seg)
				}
			case nil:
				return fmt.Errorf("simplified: structured: null element at %q", seg)
			default:
				out[seg] = el
			}
		}
	}
	return nil
}
