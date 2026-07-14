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
	return json.Marshal(flatToStructured(flat))
}

// UnmarshalStructured decodes STRUCTURED JSON into a canonical COMPOSITION
// using wt (REQ-053). It restructures to FLAT and delegates to UnmarshalFlat.
func UnmarshalStructured(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	flat, err := json.Marshal(structuredToFlat(s))
	if err != nil {
		return nil, err
	}
	return UnmarshalFlat(flat, wt)
}

// FlatToStructured restructures FLAT JSON into STRUCTURED JSON (no OPT needed).
func FlatToStructured(data []byte) ([]byte, error) {
	var flat map[string]any
	if err := json.Unmarshal(data, &flat); err != nil {
		return nil, err
	}
	return json.Marshal(flatToStructured(flat))
}

// StructuredToFlat restructures STRUCTURED JSON into FLAT JSON (no OPT needed).
func StructuredToFlat(data []byte) ([]byte, error) {
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return json.Marshal(structuredToFlat(s))
}

// flatToStructured nests a FLAT map. The first path segment (template id) is a
// single object; every deeper segment is an array indexed by its :index.
func flatToStructured(flat map[string]any) map[string]any {
	root := make(map[string]any)
	for key, val := range flat {
		pk := parseFlatKey(key)
		if len(pk.segs) == 0 {
			continue
		}
		rootID := pk.segs[0].id
		obj, _ := root[rootID].(map[string]any)
		if obj == nil {
			obj = make(map[string]any)
			root[rootID] = obj
		}
		rest := pk.segs[1:]
		if len(rest) == 0 {
			if pk.suffix != "" {
				obj["|"+pk.suffix] = val
			}
			continue
		}
		insertStructured(obj, rest, pk.suffix, val)
	}
	return root
}

// insertStructured places val at segs (relative to obj), growing arrays by
// :index. A bare leaf sets the array element to the scalar value; a suffixed
// leaf sets a |suffix key on the element object.
func insertStructured(obj map[string]any, segs []flatSeg, suffix string, val any) {
	seg := segs[0]
	idx := max(seg.idx, 0)
	arr, _ := obj[seg.id].([]any)
	for len(arr) <= idx {
		arr = append(arr, nil)
	}
	obj[seg.id] = arr

	isLeaf := len(segs) == 1
	if isLeaf && suffix == "" {
		arr[idx] = val
		return
	}
	el, ok := arr[idx].(map[string]any)
	if !ok {
		el = make(map[string]any)
		arr[idx] = el
	}
	if isLeaf {
		el["|"+suffix] = val
		return
	}
	insertStructured(el, segs[1:], suffix, val)
}

// structuredToFlat flattens a STRUCTURED map into a FLAT map. Each array
// element takes a :index; |-prefixed keys become the FLAT |suffix.
func structuredToFlat(s map[string]any) map[string]any {
	out := make(map[string]any)
	for rootID, v := range s {
		if obj, ok := v.(map[string]any); ok {
			structWalk(out, rootID, obj)
		}
	}
	return out
}

// structWalk descends an object whose keys are either child ids (mapping to
// arrays) or |-prefixed leaf suffixes, accumulating FLAT entries under path.
func structWalk(out map[string]any, path string, obj map[string]any) {
	for k, v := range obj {
		if strings.HasPrefix(k, "|") {
			out[path+k] = v
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		for i, el := range arr {
			seg := path + "/" + k + ":" + strconv.Itoa(i)
			if child, ok := el.(map[string]any); ok {
				structWalk(out, seg, child)
			} else {
				out[seg] = el
			}
		}
	}
}
