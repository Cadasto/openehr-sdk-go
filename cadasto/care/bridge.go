package care

import (
	"encoding/json"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// bridge.go — converts between the codec's canonical-JSON composition
// (map[string]any) and the typed *rm.Composition that the openEHR REST client
// requires. The datamap codec stays resource-free (map-based); this typed
// bridge lives in care, the only layer that talks to the REST client.

// compositionFromMap decodes a canonical-JSON composition (as produced by the
// codec's ToComposition) into a typed *rm.Composition via the canonical-JSON
// decoder (which dispatches polymorphic content on `_type`).
func compositionFromMap(m map[string]any) (*rm.Composition, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("care: marshal composition map: %w", err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(b, &comp); err != nil {
		return nil, fmt.Errorf("care: decode composition: %w", err)
	}
	return &comp, nil
}

// compositionToMap marshals a typed *rm.Composition back to canonical JSON as a
// map[string]any, the shape the codec's FromComposition consumes.
func compositionToMap(comp *rm.Composition) (map[string]any, error) {
	b, err := comp.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("care: marshal composition: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("care: composition to map: %w", err)
	}
	return m, nil
}
