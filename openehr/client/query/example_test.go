package query_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// resultSetJSON is the RESULT_SET the fake CDR returns: one column per
// SELECT item, and one row per composition found.
const resultSetJSON = `{
  "meta": {"_type": "RESULTSET", "_schema_version": "1.0.0"},
  "columns": [
    {"name": "uid", "path": "/uid/value"},
    {"name": "start_time", "path": "/context/start_time/value"}
  ],
  "rows": [
    ["8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::1", "2026-05-17T08:00:00Z"],
    ["90910cf0-66a0-4382-b1f8-c0f27e81b42d::sandbox.local::3", "2026-05-18T09:30:00Z"]
  ]
}`

// Execute an AQL query with the caller's data bound as a parameter, then
// read the columns and rows of the RESULT_SET.
func ExampleExecute() {
	const ehrID = "7d44b88c-4199-4bad-97dc-d78268e01398"

	// A fake CDR, run in-process by the sandbox package. It answers
	// POST /query/aql with a fixed RESULT_SET, and only when the EHR id
	// arrives in query_parameters.
	backend := sandbox.New()
	backend.HandleFunc(http.MethodPost, "/query/aql", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			QueryParameters map[string]any `json:"query_parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QueryParameters["ehr_id"] != ehrID {
			http.Error(w, "ehr_id is not bound", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resultSetJSON)
	})

	// The catalog says where the openEHR REST API lives; the transport
	// carries your *http.Client, here the one that reaches the fake.
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("https://sandbox.local/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	client, err := transport.New(catalog, transport.WithHTTPClient(backend.HTTPClient()))
	if err != nil {
		log.Fatal(err)
	}

	// $ehr_id is a placeholder. Its value travels in query_parameters,
	// beside the AQL text, so it never becomes part of the query itself.
	q := aql.Query{
		Q: "SELECT c/uid/value AS uid, c/context/start_time/value AS start_time " +
			"FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = $ehr_id",
		Parameters: map[string]any{"ehr_id": ehrID},
	}
	rs, _, err := query.Execute(context.Background(), client, q)
	if err != nil {
		log.Fatal(err)
	}

	// Columns follow the SELECT order, and so do the cells of each row.
	for _, col := range rs.Columns {
		fmt.Printf("column %s: %s\n", col.Name, col.Path)
	}
	for _, row := range rs.Rows {
		fmt.Println(row...)
	}
	// Output:
	// column uid: /uid/value
	// column start_time: /context/start_time/value
	// 8849182c-82ad-4088-a07f-48ead4180515::sandbox.local::1 2026-05-17T08:00:00Z
	// 90910cf0-66a0-4382-b1f8-c0f27e81b42d::sandbox.local::3 2026-05-18T09:30:00Z
}
