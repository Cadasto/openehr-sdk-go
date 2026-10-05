package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// resultSetFormat is the RESULT_SET the fake returns, in the shape of the
// ResultSet schema of the openEHR REST Query API: meta, q, columns and rows.
// Each temperature cell is a DV_QUANTITY object, the way a CDR returns any
// column whose path ends on an RM value. The %s slot takes the AQL text as a
// JSON string.
const resultSetFormat = `{
  "meta": {"_type": "RESULTSET", "_schema_version": "1.0.0", "_created": "2026-05-17T21:00:00Z"},
  "q": %s,
  "columns": [
    {"name": "measured", "path": "/data[at0002]/events[at0003]/time/value"},
    {"name": "temperature", "path": "/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value"}
  ],
  "rows": [
    ["2026-05-17T08:00:00Z", {"_type": "DV_QUANTITY", "magnitude": 38.1, "units": "Cel", "precision": 1}],
    ["2026-05-17T20:00:00Z", {"_type": "DV_QUANTITY", "magnitude": 37.6, "units": "Cel", "precision": 1}]
  ]
}`

// adhocQuery holds the two members of the AdhocQueryExecute body that the
// fake reads; it ignores offset and fetch.
type adhocQuery struct {
	Q               string         `json:"q"`
	QueryParameters map[string]any `json:"query_parameters"`
}

// placeholder finds the $name parameters in AQL text.
var placeholder = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// newFakeCDR returns an in-memory backend that answers POST /query/aql. The
// sandbox package matches the route after the deployment's REST base, so a
// request to /openehr/v1/query/aql reaches it.
func newFakeCDR() *sandbox.Backend {
	backend := sandbox.New()
	backend.HandleFunc(http.MethodPost, "/query/aql", answerQuery)
	return backend
}

// answerQuery is the whole fake. It does not evaluate AQL: it checks how the
// parameters travelled, then returns the same two readings.
func answerQuery(w http.ResponseWriter, r *http.Request) {
	var body adhocQuery
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, "INVALID_REQUEST", "the body is not an AdhocQueryExecute document")
		return
	}
	// Like a real CDR: every $name in the text needs a value.
	for _, match := range placeholder.FindAllStringSubmatch(body.Q, -1) {
		if _, ok := body.QueryParameters[match[1]]; !ok {
			writeError(w, "AQL_PARAMETER_MISSING", "query parameter $"+match[1]+" has no value")
			return
		}
	}
	// Stricter than a real CDR, so the example proves the binding: the EHR id
	// and the threshold arrive in query_parameters, and the text holds only
	// their placeholders.
	params := body.QueryParameters
	if params["ehr_id"] != patientEHRID || params["min_temp"] != feverThreshold ||
		!strings.Contains(body.Q, "$ehr_id") || !strings.Contains(body.Q, "$min_temp") ||
		strings.Contains(body.Q, patientEHRID) {
		writeError(w, "AQL_PARAMETER_INVALID", "the EHR id and threshold were not bound as parameters")
		return
	}
	q, err := json.Marshal(body.Q)
	if err != nil {
		http.Error(w, "encode result", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// A write failure here would surface as a decode error on the client side.
	_, _ = fmt.Fprintf(w, resultSetFormat, q)
}

// writeError answers 400 Bad Request with an openEHR error envelope shaped
// like the vendored sample: a message, a code, and the code as coded text.
func writeError(w http.ResponseWriter, code, message string) {
	body, err := json.Marshal(map[string]any{
		"message": message,
		"code":    code,
		"coded_text": []any{map[string]any{
			"terminology_id": map[string]string{"value": "local"},
			"code_string":    code,
		}},
	})
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(body)
}
