// This example executes an AQL query and reads its result, picking up where
// aql-build stops: bind caller data as parameters, send the query with
// query.Execute, print the RESULT_SET, turn an RM-valued cell into a typed
// rm.DVQuantity, and classify a query the server refuses.
//
// It runs offline: an in-process fake clinical data repository (CDR), built
// on the sandbox package, answers POST /query/aql with fixed values and no
// network listener.
//
// Run:
//
//	go run ./cmd/examples/query-execute
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The query reads body-temperature OBSERVATIONs. Its paths start at the alias
// "o" and reach the event time and the measured temperature, a DV_QUANTITY.
const (
	bodyTemperature = "openEHR-EHR-OBSERVATION.body_temperature.v2"
	timePath        = "o/data[at0002]/events[at0003]/time/value"
	temperaturePath = "o/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value"
)

// The caller's data: the EHR to read, which is the one EHR the fake CDR holds
// readings for, and the lowest temperature to return, in degrees Celsius.
const (
	patientEHRID   = "7d44b88c-4199-4bad-97dc-d78268e01398"
	feverThreshold = 37.5
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: give the whole run a deadline and inject the HTTP client. The
	// sandbox client reaches the fake CDR without a listener; an application
	// injects its own *http.Client the same way, with its own timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpClient := newFakeCDR().HTTPClient()
	httpClient.Timeout = 10 * time.Second
	client, err := newClient(httpClient)
	if err != nil {
		return err
	}

	// Step 2: build the query and bind the caller's data. Bind puts each
	// value in query_parameters, beside the text: the value never becomes
	// AQL, so it cannot change what the query means the way a pasted string
	// can. Limit sets fetch in the request body, not LIMIT in the text.
	q, err := temperatureQuery().
		Bind("ehr_id", patientEHRID).
		Bind("min_temp", feverThreshold).
		Limit(10).
		Build()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}
	fmt.Println("query:", q)
	fmt.Println("bound parameters:", strings.Join(slices.Sorted(maps.Keys(q.Parameters)), ", "))
	fmt.Println("fetch:", q.Fetch)

	// Step 3: execute. query.Execute posts the query to /query/aql and
	// decodes the RESULT_SET. The second result is the response metadata,
	// such as the ETag.
	rs, _, err := query.Execute(ctx, client, q)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	// Step 4: read the result. Columns follow the SELECT order, and so do
	// the cells of each row. A cell decodes with encoding/json into any: text
	// arrives as a string, a JSON number in a plain cell as float64, and an
	// RM value as a map[string]any carrying its "_type".
	fmt.Println("columns:")
	for _, col := range rs.Columns {
		fmt.Printf("  %-12s %s\n", col.Name, col.Path)
	}
	fmt.Printf("rows: %d\n", len(rs.Rows))
	for _, row := range rs.Rows {
		if len(row) != len(rs.Columns) {
			return fmt.Errorf("row has %d cells for %d columns", len(row), len(rs.Columns))
		}
		temperature, err := quantityCell(row[1])
		if err != nil {
			return err
		}
		fmt.Printf("  %v  %g %s\n", row[0], temperature.Magnitude, temperature.Units)
	}

	// Step 5: classify a query the server refuses.
	return showRefusal(ctx, client)
}

// newClient wires the transport the way every REST example does: a static
// service catalog names the openEHR REST base URL, and transport.New takes
// the injected *http.Client. The SDK never allocates one of its own.
func newClient(httpClient *http.Client) (*transport.Client, error) {
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
		return nil, fmt.Errorf("build static catalog: %w", err)
	}
	client, err := transport.New(catalog, transport.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("transport.New: %w", err)
	}
	return client, nil
}

// temperatureQuery prepares the body-temperature readings at or above
// $min_temp in the EHR named by $ehr_id. It binds nothing, so each caller
// decides what travels with the placeholders.
func temperatureQuery() *aql.Builder {
	return aql.NewBuilder().
		Select(aql.ColAs(timePath, "measured"), aql.ColAs(temperaturePath, "temperature")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", "o", bodyTemperature)).
		Where(aql.Ge(temperaturePath+"/magnitude", aql.Param("min_temp")))
}

// quantityCell turns an RM-valued cell into a typed DV_QUANTITY. The cell
// arrived as a map, so it goes back to JSON first; the SDK's type registry
// then decodes it by its "_type" and refuses a cell of any other RM type.
func quantityCell(cell aql.ResultCell) (*rm.DVQuantity, error) {
	raw, err := json.Marshal(cell)
	if err != nil {
		return nil, fmt.Errorf("re-encode cell: %w", err)
	}
	quantity, err := typereg.DecodeAs[*rm.DVQuantity](raw)
	if err != nil {
		return nil, fmt.Errorf("decode DV_QUANTITY cell: %w", err)
	}
	return quantity, nil
}

// showRefusal sends the same query with the Bind calls left out. The text
// still names $ehr_id and $min_temp, but no value travels with them, so the
// fake CDR answers 400 with an openEHR error envelope.
func showRefusal(ctx context.Context, client *transport.Client) error {
	unbound, err := temperatureQuery().Limit(10).Build()
	if err != nil {
		return fmt.Errorf("build unbound query: %w", err)
	}
	_, _, err = query.Execute(ctx, client, unbound)
	if err == nil {
		return errors.New("the unbound query was accepted; expected a refusal")
	}
	// query.Execute returns a *query.AQLError for a refusal whose openEHR
	// error envelope carries a code, and for any 400, 408 or 501 without
	// one. Code is the server's own code. The server's message can quote
	// patient data, so the transport drops it unless the client opts in with
	// transport.WithRawErrorBodies(true); print a fixed text instead. A 501
	// also matches errors.Is(err, aql.ErrEngineCapability): valid AQL that
	// this deployment does not implement.
	aqlErr, ok := errors.AsType[*query.AQLError](err)
	if !ok || aqlErr == nil {
		return fmt.Errorf("execute unbound query: %w", err)
	}
	fmt.Println("query without Bind calls:")
	fmt.Printf("  refused by the CDR, AQL error code %s\n", aqlErr.Code)
	// The AQL error wraps the transport's wire error, which keeps the status.
	if wireErr, ok := errors.AsType[*transport.WireError](err); ok && wireErr != nil {
		fmt.Printf("  HTTP status %d\n", wireErr.StatusCode)
	}
	return nil
}
